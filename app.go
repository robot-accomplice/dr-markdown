package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	imageassets "dr-markdown/internal/assets"
	"dr-markdown/internal/document"
	"dr-markdown/internal/eventlog"
	"dr-markdown/internal/fonts"
	"dr-markdown/internal/preferences"
	"dr-markdown/internal/session"
)

//go:embed VERSION
var versionFile string

// appVersion is the build identity carried into every recorded event, so a
// user's bug report can be tied to what actually ran.
//
// It is READ from the VERSION file rather than written here. A version that
// exists in two places is a version that drifts, and this project has already
// shipped that bug: the plist said 1.0.0 for every release ever made, because
// nothing compared it to anything. VERSION is the single source — Go embeds it,
// the packaging script reads it, and it outlives whatever builds the bundle.
var appVersion = strings.TrimSpace(versionFile)

// fileOpenEvent is the host event carrying a path macOS asked us to open while
// the app was already running. The frontend subscribes to it by this name.
const fileOpenEvent = "file:open"

var iconFreeMessageDialogPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x60, 0x60, 0x60, 0x60,
	0x00, 0x00, 0x00, 0x05, 0x00, 0x01, 0xa5, 0xf6,
	0x45, 0x40,
	0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
	0xae, 0x42, 0x60, 0x82,
}

// App is the host-bound API exposed to the frontend as globalThis.drmd.native.
type App struct {
	ctx context.Context

	// mu guards pendingOpen/frontendReady only.
	mu sync.Mutex
	// session owns the open tabs, their dirty state and the on-disk baseline.
	session *session.Session
	// pendingOpen holds files macOS handed us before the webview could accept
	// one. Launching by double-click delivers the file BEFORE the frontend
	// exists, so emitting straight away would drop it silently — which is the
	// defect this replaced (#53).
	pendingOpen []string
	// frontendReady flips once the webview has asked for its pending files.
	frontendReady bool
	// documents owns open, save and resolve-unsaved, including the
	// currentPath/currentText state those operations mutate. (Not named "docs":
	// TestAppDoesNotDuplicateSessionState guards that name against App
	// re-declaring the open tabs the session owns.)
	documents      *documentUseCase
	native         nativePort
	fonts          fontPort
	preferences    preferencePort
	images         *imageUseCase
	links          *linkUseCase
	defaultHandler *defaultHandlerUseCase
	events         *eventlog.Log
	// panicDialog fires at most once. Deliberately not guarded by mu: a panic
	// raised while mu is held would deadlock its own report. See reportPanic.
	panicDialog sync.Once
	// blockedNav dedupes the navigation refusals already recorded. It has its
	// own mutex because it is written from the host's callback, which does not
	// hold mu and must not wait on it.
	blockedNavMu sync.Mutex
	blockedNav   map[string]bool
}

type appDependencies struct {
	native      nativePort
	documents   documentPort
	fonts       fontPort
	preferences preferencePort
	images      imageAssetPort
	events      *eventlog.Log
}

type nativePort interface {
	OpenMarkdownFile(context.Context) (string, error)
	SaveMarkdownFile(context.Context, string) (string, error)
	SelectImageFile(context.Context) (string, error)
	RevealPath(context.Context, string) error
	ConfirmOverwriteChanged(context.Context, string) (string, error)
	OpenExternalURL(context.Context, string) error
	// SubscribeFileDrop receives paths from the OS. Emitting them to the webview
	// is EmitFilesDropped, deliberately separate: the two cross the boundary in
	// opposite directions and are different mechanisms under any other host.
	SubscribeFileDrop(context.Context, func(paths []string))
	EmitFilesDropped(context.Context, []string)
	ShowError(context.Context, string, string)
	ConfirmUnsaved(context.Context) (string, error)
	SetTitle(context.Context, string)
	// The default-handler offer (docs/decisions/2026-08-25-default-markdown-handler.md).
	// Set means ASK: macOS shows its own consent dialog and applies the change
	// only if the user confirms, and the call returns success while the question
	// is still unanswered — so a nil error means "asked", never "set".
	IsDefaultMarkdownHandler(context.Context) (bool, error)
	SetDefaultMarkdownHandler(context.Context) error
	EmitFileOpen(context.Context, string)
}

type documentPort interface {
	ReadMarkdown(path string) (string, error)
	WriteMarkdown(path string, content string) error
}

type fontPort interface {
	ListFamilies() []string
}

type preferencePort interface {
	Load() (preferences.Preferences, error)
	Save(preferences.Preferences) error
	RecordRecent(path string) ([]preferences.RecentDocument, error)
}

type imageAssetPort interface {
	ImportForDocument(documentPath string, sourcePath string) (imageassets.ImportedImage, error)
	LoadForDocument(documentPath string, markdownPath string) (imageassets.LoadedImage, error)
}

// NewApp takes its native operations rather than constructing them, so this
// file names no host. main.go asks the host for them.
// NewEventLog opens the trail, separately from the App and BEFORE it.
//
// It used to be constructed inside NewApp, which put every line of NewApp
// before it in a window where a panic left nothing at all: no record, no
// dialog, and a process that simply stopped (#62). Opening the trail first is
// the only way to cover the construction of the thing that owns it.
//
// It resolves its own directory and cannot fail: os.UserConfigDir falls back to
// the temp directory, the same fallback NewApp already applied, and every
// failure to write is swallowed by the log itself.
func NewEventLog() *eventlog.Log {
	logDir, err := os.UserConfigDir()
	if err != nil {
		logDir = os.TempDir()
	}
	// The directory keeps the ORIGINAL name. Renaming it to match the
	// application would abandon every existing trail on an upgrade, and this
	// exists to make a past failure investigable. The user never sees it.
	return eventlog.New(filepath.Join(logDir, "Dr. Markdown"), appVersion, time.Now)
}

// NewApp builds the application around an already-open trail, so a panic during
// construction has somewhere to be recorded.
func NewApp(native nativePort, events *eventlog.Log) *App {
	store, err := preferences.DefaultStore()
	if err != nil {
		store = preferences.NewStore(filepath.Join(os.TempDir(), "Dr. Markdown"), time.Now)
	}
	return newAppWithDependencies(appDependencies{
		events:      events,
		native:      native,
		documents:   documentAdapter{},
		fonts:       fontAdapter{},
		preferences: store,
		images:      imageAssetAdapter{},
	})
}

func newAppWithDependencies(deps appDependencies) *App {
	sess := &session.Session{}
	return &App{
		session:        sess,
		events:         deps.events,
		native:         deps.native,
		fonts:          deps.fonts,
		preferences:    deps.preferences,
		images:         &imageUseCase{native: deps.native, images: deps.images},
		links:          &linkUseCase{native: deps.native},
		defaultHandler: &defaultHandlerUseCase{native: deps.native},
		documents: &documentUseCase{
			native:      deps.native,
			documents:   deps.documents,
			preferences: deps.preferences,
			events:      deps.events,
			session:     sess,
		},
	}
}

func (a *App) startup(ctx context.Context) {
	defer a.reportPanic("startup")

	a.ctx = ctx
	// The host resolves dropped files to real filesystem paths; the frontend's DOM
	// drop event cannot. Subscribing through the native port keeps startup
	// callable from tests that have no host runtime context.
	//
	// Receiving and forwarding are two calls on purpose. One takes paths FROM the
	// OS, the other sends them TO the webview, and under a different host they are
	// different mechanisms — a single fused call hid that.
	a.native.SubscribeFileDrop(ctx, func(paths []string) {
		a.native.EmitFilesDropped(ctx, paths)
	})
}

// OpenResult is returned to the frontend when a document is opened.
type OpenResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// OpenDocument shows a native open dialog and reads the chosen file.
// A canceled dialog returns an empty OpenResult and nil error.
func (a *App) OpenDocument() (OpenResult, error) {
	defer a.reportPanic("OpenDocument")
	return a.documents.openViaDialog(a.ctx)
}

// SaveDocument writes content to path atomically. path must be non-empty.
//
// Before overwriting, it checks that the file on disk still holds the bytes the
// app last saw there. Nothing did that before, so a change made by anything else
// — a git pull, a sync client, a second window — was replaced with no error and
// no prompt.
func (a *App) SaveDocument(path, content string) error {
	defer a.reportPanic("SaveDocument")
	return a.documents.save(a.ctx, path, content)
}

// SaveDocumentAs shows a native save dialog and writes content atomically.
// Returns the saved path, or "" if the user canceled.
func (a *App) SaveDocumentAs(content string) (string, error) {
	defer a.reportPanic("SaveDocumentAs")
	return a.documents.saveAs(a.ctx, content)
}

// OpenDocument is one editor tab as the frontend sees it. Go does not infer
// which document is current; the frontend names it on every push.
// Aliased rather than redefined so the binding signature and the JSON
// wire format are byte-identical to before the session extraction — the json
// tags are the contract with the webview.
type OpenDocument = session.Document

// SyncDocuments replaces Go's view of the open tabs.
//
// This exists because Go used to hold a single ambient (currentPath,
// currentText). The frontend is multi-tab and never reset the path when a new
// tab opened, so the close guard wrote the new tab's text over the previously
// opened FILE — silently destroying a document the user had not touched. Any
// write must now name its own target, and dirty state aggregates across tabs
// rather than tracking only the visible one.
func (a *App) SyncDocuments(docs []OpenDocument) {
	defer a.reportPanic("SyncDocuments")

	a.session.Sync(docs)
	a.documents.updateTitle(a.ctx)
}

// SetDirty records the frontend's dirty state for the active tab.
//
// With no synced documents the flag is kept on its own rather than invented
// against the last opened path: not knowing which file is dirty must lead to
// asking the user, never to writing a guess.
func (a *App) SetDirty(dirty bool) {
	defer a.reportPanic("SetDirty")

	a.session.SetDirty(dirty)
	a.documents.updateTitle(a.ctx)
}

// UpdateContent stores the latest markdown for the active tab (pushed
// debounced by the frontend) so the close guard can save without a round-trip.
func (a *App) UpdateContent(content string) {
	defer a.reportPanic("UpdateContent")
	a.session.UpdateActiveContent(content)
}

// ListFontFamilies returns installed font family names for settings controls.
func (a *App) ListFontFamilies() []string {
	defer a.reportPanic("ListFontFamilies")

	return a.fonts.ListFamilies()
}

// LoadPreferences returns persisted preferences and recents for frontend boot.
func (a *App) LoadPreferences() (preferences.Preferences, error) {
	defer a.reportPanic("LoadPreferences")

	return a.preferences.Load()
}

// SavePreferences persists runtime settings selected in the frontend.
func (a *App) SavePreferences(prefs preferences.Preferences) error {
	defer a.reportPanic("SavePreferences")

	return a.preferences.Save(prefs)
}

// OpenRecentDocument opens a known recent path without showing a native picker.
func (a *App) OpenRecentDocument(path string) (OpenResult, error) {
	defer a.reportPanic("OpenRecentDocument")
	if path == "" {
		return OpenResult{}, fmt.Errorf("OpenRecentDocument: empty path")
	}
	return a.documents.open(a.ctx, path)
}

// ImportImage selects an image, copies it into the document asset folder, and
// returns markdown for insertion. A canceled picker returns an empty result.
// An unsaved document is rejected before the picker opens, so the user is
// never asked to choose a file the import could never have accepted.
func (a *App) ImportImage(documentPath string) (imageassets.ImportedImage, error) {
	defer a.reportPanic("ImportImage")
	return a.images.importViaPicker(a.ctx, documentPath)
}

// ImportDroppedImage imports a file the user dropped onto the window. The
// path is already known, so no picker is shown, but the same asset policy and
// unsaved-document rejection apply as for the ribbon command.
func (a *App) ImportDroppedImage(documentPath string, sourcePath string) (imageassets.ImportedImage, error) {
	defer a.reportPanic("ImportDroppedImage")
	return a.images.importDropped(a.ctx, documentPath, sourcePath)
}

// LoadImageAsset inlines a document-relative image so the webview can render
// it and so print/export artifacts stay self-contained.
func (a *App) LoadImageAsset(documentPath string, markdownPath string) (imageassets.LoadedImage, error) {
	defer a.reportPanic("LoadImageAsset")
	return a.images.load(a.ctx, documentPath, markdownPath)
}

// OpenExternalURL opens a web link in the user's browser.
//
// Without it, clicking a link in the preview navigated the app's own window to
// the remote page — a chrome-less window with no address bar and no back
// button, from which the only escape is quitting.
func (a *App) OpenExternalURL(raw string) error {
	defer a.reportPanic("OpenExternalURL")
	return a.links.openExternal(a.ctx, raw)
}

// RecordClientEvent lets the frontend put a diagnostic into the same trail as
// the Go side. Frontend failures were console warnings, and a production build
// has no devtools, so nothing the webview reported ever reached anyone.
//
// Fields are recorded as data, never interpreted. The webview parses untrusted
// document content, so anything arriving here is untrusted too.
func (a *App) RecordClientEvent(event string, fields map[string]string) {
	defer a.reportPanic("RecordClientEvent")

	if event == "" {
		return
	}
	a.events.Record("client."+event, fields)
}

// RevealImageAsset shows an image asset in the OS file browser. A missing
// asset is reported instead of silently doing nothing.
func (a *App) RevealImageAsset(documentPath string, markdownPath string) error {
	defer a.reportPanic("RevealImageAsset")
	return a.images.reveal(a.ctx, documentPath, markdownPath)
}

// SetAsDefaultMarkdownHandler backs the application-menu offer. The menu item
// is disabled in exactly the two states this refuses, but menu state is a
// rendering of a decision, not the decision — a stale menu must not be able to
// set a handler from a disk image, so the guards run again here.
func (a *App) SetAsDefaultMarkdownHandler() {
	defer a.reportPanic("SetAsDefaultMarkdownHandler")
	a.defaultHandler.setAsDefault(a.ctx)
}

// ResolveUnsavedChanges reports whether the frontend may discard the
// current dirty buffer (e.g. to open another document). Not dirty: true
// immediately. Otherwise it shows the same Save / Don't Save / Cancel
// dialog as the close guard; Save saves first, Don't Save proceeds, and
// Cancel (or a dialog/save failure) aborts.
func (a *App) ResolveUnsavedChanges() bool {
	defer a.reportPanic("ResolveUnsavedChanges")
	return a.documents.resolveUnsaved(a.ctx)
}

// beforeClose implements the unsaved-changes guard: Save / Don't Save /
// Cancel, matching the spec's error-handling contract.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	defer a.reportPanic("beforeClose")
	return a.documents.preventClose(ctx)
}

type documentAdapter struct{}

func (documentAdapter) ReadMarkdown(path string) (string, error) {
	return document.Read(path)
}

func (documentAdapter) WriteMarkdown(path string, content string) error {
	return document.WriteAtomic(path, content)
}

type fontAdapter struct{}

func (fontAdapter) ListFamilies() []string {
	return fonts.ListFamilies(os.Getenv("HOME"))
}

type imageAssetAdapter struct{}

func (imageAssetAdapter) ImportForDocument(documentPath string, sourcePath string) (imageassets.ImportedImage, error) {
	return imageassets.ImportForDocument(documentPath, sourcePath)
}

func (imageAssetAdapter) LoadForDocument(documentPath string, markdownPath string) (imageassets.LoadedImage, error) {
	return imageassets.LoadForDocument(documentPath, markdownPath)
}

// openFileFromOS receives a path macOS routed to us: a double-click in Finder, a
// drop on the Dock icon, or `open -a`. The bundle advertises the association
// through CFBundleDocumentTypes, so the file arrives whether or not anything
// consumes it — for a long time nothing did, and the user got an empty document
// with no hint their file had gone (#53).
//
// Before the frontend is listening the path is held rather than emitted, because
// the launch case delivers the file first and an event into a webview that does
// not exist yet is an event nobody receives.
func (a *App) openFileFromOS(path string) {
	defer a.reportPanic("openFileFromOS")

	if path == "" {
		return
	}
	a.mu.Lock()
	ready := a.frontendReady
	if !ready {
		a.pendingOpen = append(a.pendingOpen, path)
	}
	a.mu.Unlock()
	if ready {
		a.native.EmitFileOpen(a.ctx, path)
	}
}

// FrontendReady is called by the webview once it can accept a document. It
// returns every file that arrived while nothing was listening, and clears them:
// the frontend calls this on every boot, and a reload must not reopen a document
// the user has already closed.
func (a *App) FrontendReady() []string {
	defer a.reportPanic("FrontendReady")

	a.mu.Lock()
	defer a.mu.Unlock()
	a.frontendReady = true
	pending := a.pendingOpen
	a.pendingOpen = nil
	return pending
}

// recordBlockedNavigation records a main-frame navigation the host refused.
//
// The host cancels it in the navigation delegate, where WebKit asks; this is
// what makes the refusal reviewable afterwards. It is capped and deduped for the
// same reason a refused link scheme is: the URL is attacker-controlled content,
// and a refusal recorded on every attempt lets a document the app has ALREADY
// judged hostile evict the rest of the trail.
func (a *App) recordBlockedNavigation(url string) {
	a.blockedNavMu.Lock()
	if a.blockedNav == nil {
		a.blockedNav = map[string]bool{}
	}
	_, seen := a.blockedNav[url]
	if !seen && len(a.blockedNav) < blockedNavigationRecordCap {
		a.blockedNav[url] = true
	}
	a.blockedNavMu.Unlock()
	if seen {
		return
	}
	a.events.Record("navigation.blocked", map[string]string{"url": url})
}

// The cap bounds the same attack carried out with many distinct URLs rather
// than one repeated URL.
const blockedNavigationRecordCap = 32
