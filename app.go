package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	runtime2 "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	imageassets "dr-markdown/internal/assets"
	"dr-markdown/internal/document"
	"dr-markdown/internal/eventlog"
	"dr-markdown/internal/fonts"
	"dr-markdown/internal/preferences"
	"dr-markdown/internal/session"
)

// appVersion is the build identity carried into every recorded event, so a
// user's bug report can be tied to what actually ran. Kept in step with
// wails.json by TestAppVersionMatchesWailsConfig.
const appVersion = "0.4.1"

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

// App is the Wails-bound API exposed to the frontend as window.go.main.App.
type App struct {
	ctx context.Context

	mu sync.Mutex
	// session owns the open tabs, their dirty state and the on-disk baseline.
	session *session.Session
	// currentPath is the last path opened or saved. It names the window title
	// and the Save-As default only; it is NEVER a write target, because
	// inferring the target from ambient state is what destroyed documents.
	currentPath string
	currentText string
	// onDisk records, per path, the bytes the app last read from or wrote to
	// that file. It is the baseline the staleness check compares against; it is
	// never a write target.
	onDisk      map[string]string
	native      nativePort
	documents   documentPort
	fonts       fontPort
	preferences preferencePort
	images      imageAssetPort
	events      *eventlog.Log
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
	SubscribeFileDrop(context.Context)
	ShowError(context.Context, string, string)
	ConfirmUnsaved(context.Context) (string, error)
	SetTitle(context.Context, string)
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

func NewApp() *App {
	store, err := preferences.DefaultStore()
	if err != nil {
		store = preferences.NewStore(filepath.Join(os.TempDir(), "Dr. Markdown"), time.Now)
	}
	logDir, err := os.UserConfigDir()
	if err != nil {
		logDir = os.TempDir()
	}
	return newAppWithDependencies(appDependencies{
		events:      eventlog.New(filepath.Join(logDir, "Dr. Markdown"), appVersion, time.Now),
		native:      wailsNative{},
		documents:   documentAdapter{},
		fonts:       fontAdapter{},
		preferences: store,
		images:      imageAssetAdapter{},
	})
}

func newAppWithDependencies(deps appDependencies) *App {
	return &App{
		session:     &session.Session{},
		events:      deps.events,
		native:      deps.native,
		documents:   deps.documents,
		fonts:       deps.fonts,
		preferences: deps.preferences,
		images:      deps.images,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Wails resolves dropped files to real filesystem paths; the frontend's DOM
	// drop event cannot. Subscribing through the native port keeps startup
	// callable from tests that have no Wails runtime context.
	a.native.SubscribeFileDrop(ctx)
}

// OpenResult is returned to the frontend when a document is opened.
type OpenResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// OpenDocument shows a native open dialog and reads the chosen file.
// A canceled dialog returns an empty OpenResult and nil error.
func (a *App) OpenDocument() (OpenResult, error) {
	path, err := a.native.OpenMarkdownFile(a.ctx)
	if err != nil {
		return OpenResult{}, err
	}
	if path == "" {
		return OpenResult{}, nil
	}
	return a.openPath(path)
}

// SaveDocument writes content to path atomically. path must be non-empty.
//
// Before overwriting, it checks that the file on disk still holds the bytes the
// app last saw there. Nothing did that before, so a change made by anything else
// — a git pull, a sync client, a second window — was replaced with no error and
// no prompt.
func (a *App) SaveDocument(path, content string) error {
	if path == "" {
		return fmt.Errorf("SaveDocument: empty path")
	}
	if err := a.confirmNoExternalChange(path); err != nil {
		return err
	}
	if err := a.documents.WriteMarkdown(path, content); err != nil {
		a.events.Record("document.save.failed", map[string]string{"path": path, "error": err.Error()})
		a.native.ShowError(a.ctx, "Save Failed", err.Error())
		return err
	}
	a.events.Record("document.saved", map[string]string{"path": path, "bytes": strconv.Itoa(len(content))})
	a.mu.Lock()
	a.currentPath = path
	a.currentText = content
	a.rememberOnDisk(path, content)
	a.mu.Unlock()
	a.session.AdoptPath(path, content)
	a.recordRecent(path)
	a.updateTitle()
	return nil
}

// SaveDocumentAs shows a native save dialog and writes content atomically.
// Returns the saved path, or "" if the user canceled.
func (a *App) SaveDocumentAs(content string) (string, error) {
	path, err := a.native.SaveMarkdownFile(a.ctx, "untitled.md")
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	if err := a.SaveDocument(path, content); err != nil {
		return "", err
	}
	return path, nil
}

// OpenDocument is one editor tab as the frontend sees it. Go does not infer
// which document is current; the frontend names it on every push.
// Aliased rather than redefined so the Wails binding signature and the JSON
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
	a.session.Sync(docs)
	a.updateTitle()
}

// SetDirty records the frontend's dirty state for the active tab.
//
// With no synced documents the flag is kept on its own rather than invented
// against the last opened path: not knowing which file is dirty must lead to
// asking the user, never to writing a guess.
func (a *App) SetDirty(dirty bool) {
	a.session.SetDirty(dirty)
	a.updateTitle()
}

// dirtyDocuments returns a copy of every tab with unsaved changes.
func (a *App) dirtyDocuments() []OpenDocument { return a.session.Dirty() }

// activeDocument returns the focused tab, or the first, or a zero value.
func (a *App) activeDocument() OpenDocument { return a.session.Active() }

// UpdateContent stores the latest markdown for the active tab (pushed
// debounced by the frontend) so the close guard can save without a round-trip.
func (a *App) UpdateContent(content string) { a.session.UpdateActiveContent(content) }

// ListFontFamilies returns installed font family names for settings controls.
func (a *App) ListFontFamilies() []string {
	return a.fonts.ListFamilies()
}

// LoadPreferences returns persisted preferences and recents for frontend boot.
func (a *App) LoadPreferences() (preferences.Preferences, error) {
	return a.preferences.Load()
}

// SavePreferences persists runtime settings selected in the frontend.
func (a *App) SavePreferences(prefs preferences.Preferences) error {
	return a.preferences.Save(prefs)
}

// OpenRecentDocument opens a known recent path without showing a native picker.
func (a *App) OpenRecentDocument(path string) (OpenResult, error) {
	if path == "" {
		return OpenResult{}, fmt.Errorf("OpenRecentDocument: empty path")
	}
	return a.openPath(path)
}

// errUnsavedImageImport rejects imports that cannot produce a portable
// relative asset path because the document has no location on disk yet.
var errUnsavedImageImport = errors.New("Save the document before inserting images.")

// ImportImage selects an image, copies it into the document asset folder, and
// returns markdown for insertion. A canceled picker returns an empty result.
// An unsaved document is rejected before the picker opens, so the user is
// never asked to choose a file the import could never have accepted.
func (a *App) ImportImage(documentPath string) (imageassets.ImportedImage, error) {
	if documentPath == "" {
		a.native.ShowError(a.ctx, "Image Import Failed", errUnsavedImageImport.Error())
		return imageassets.ImportedImage{}, errUnsavedImageImport
	}
	sourcePath, err := a.native.SelectImageFile(a.ctx)
	if err != nil {
		return imageassets.ImportedImage{}, err
	}
	if sourcePath == "" {
		return imageassets.ImportedImage{}, nil
	}
	result, err := a.images.ImportForDocument(documentPath, sourcePath)
	if err != nil {
		a.native.ShowError(a.ctx, "Image Import Failed", err.Error())
		return imageassets.ImportedImage{}, err
	}
	return result, nil
}

// ImportDroppedImage imports a file the user dropped onto the window. The
// path is already known, so no picker is shown, but the same asset policy and
// unsaved-document rejection apply as for the ribbon command.
func (a *App) ImportDroppedImage(documentPath string, sourcePath string) (imageassets.ImportedImage, error) {
	if documentPath == "" {
		a.native.ShowError(a.ctx, "Image Import Failed", errUnsavedImageImport.Error())
		return imageassets.ImportedImage{}, errUnsavedImageImport
	}
	result, err := a.images.ImportForDocument(documentPath, sourcePath)
	if err != nil {
		a.native.ShowError(a.ctx, "Image Import Failed", err.Error())
		return imageassets.ImportedImage{}, err
	}
	return result, nil
}

// LoadImageAsset inlines a document-relative image so the webview can render
// it and so print/export artifacts stay self-contained.
func (a *App) LoadImageAsset(documentPath string, markdownPath string) (imageassets.LoadedImage, error) {
	return a.images.LoadForDocument(documentPath, markdownPath)
}

// RevealImageAsset shows an image asset in the OS file browser. A missing
// asset is reported instead of silently doing nothing.
// safeExternalSchemes is the Go-side allowlist for opening a URL in the user's
// browser. It deliberately duplicates the frontend check rather than trusting
// it: the webview is where untrusted document content is parsed, so a bound
// method that hands any string to the OS URL opener is a second route to the
// execution the frontend check exists to prevent — and the OS opener will
// happily launch a registered local handler for a scheme a browser would never
// navigate to.
var safeExternalSchemes = map[string]bool{"http": true, "https": true, "mailto": true}

// OpenExternalURL opens a web link in the user's browser.
//
// Without it, clicking a link in the preview navigated the app's own window to
// the remote page — a chrome-less window with no address bar and no back
// button, from which the only escape is quitting.
func (a *App) OpenExternalURL(raw string) error {
	// Strip exactly what a URL parser strips, so this check cannot be fooled by
	// a string that reads as harmless here and as `javascript:` to the opener.
	cleaned := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, raw)
	cleaned = strings.TrimFunc(cleaned, func(r rune) bool { return r <= ' ' })

	parsed, err := url.Parse(cleaned)
	if err != nil {
		return fmt.Errorf("open link: %q is not a URL", raw)
	}
	if !safeExternalSchemes[strings.ToLower(parsed.Scheme)] {
		return fmt.Errorf("open link: refusing scheme %q", parsed.Scheme)
	}
	return a.native.OpenExternalURL(a.ctx, cleaned)
}

// rememberOnDisk records the bytes now believed to be on disk at path.
// Callers must hold a.mu.
func (a *App) rememberOnDisk(path, content string) {
	if a.onDisk == nil {
		a.onDisk = map[string]string{}
	}
	a.onDisk[path] = content
}

// confirmNoExternalChange refuses a save that would overwrite a change the app
// never saw, unless the user explicitly chooses to overwrite.
//
// It compares against what the app last READ OR WROTE, not against what it
// first opened — otherwise every second save to the same file would look like
// an external edit and the prompt would become noise the user learns to click
// through, which is worse than no prompt at all.
//
// A path the app has never touched has no baseline and is saved without
// interruption; so is one that cannot be re-read, because failing to verify is
// not evidence of a conflict and must not block the user from saving their work.
func (a *App) confirmNoExternalChange(path string) error {
	a.mu.Lock()
	expected, known := a.onDisk[path]
	a.mu.Unlock()
	if !known {
		return nil
	}
	current, err := a.documents.ReadMarkdown(path)
	if err != nil || current == expected {
		return nil
	}
	choice, err := a.native.ConfirmOverwriteChanged(a.ctx, path)
	if err != nil {
		return err
	}
	a.events.Record("document.conflict", map[string]string{"path": path, "choice": choice})
	if choice != "Overwrite" {
		return fmt.Errorf("save canceled: %s changed on disk since it was opened", filepath.Base(path))
	}
	return nil
}

// RecordClientEvent lets the frontend put a diagnostic into the same trail as
// the Go side. Frontend failures were console warnings, and a production build
// has no devtools, so nothing the webview reported ever reached anyone.
//
// Fields are recorded as data, never interpreted. The webview parses untrusted
// document content, so anything arriving here is untrusted too.
func (a *App) RecordClientEvent(event string, fields map[string]string) {
	if event == "" {
		return
	}
	a.events.Record("client."+event, fields)
}

func (a *App) RevealImageAsset(documentPath string, markdownPath string) error {
	loaded, err := a.images.LoadForDocument(documentPath, markdownPath)
	if err != nil {
		a.native.ShowError(a.ctx, "Reveal Failed", err.Error())
		return err
	}
	if !loaded.Exists {
		err := fmt.Errorf("image asset is missing: %s", markdownPath)
		a.native.ShowError(a.ctx, "Reveal Failed", err.Error())
		return err
	}
	if err := a.native.RevealPath(a.ctx, loaded.AbsolutePath); err != nil {
		a.native.ShowError(a.ctx, "Reveal Failed", err.Error())
		return err
	}
	return nil
}

// ResolveUnsavedChanges reports whether the frontend may discard the
// current dirty buffer (e.g. to open another document). Not dirty: true
// immediately. Otherwise it shows the same Save / Don't Save / Cancel
// dialog as the close guard; Save saves first, Don't Save proceeds, and
// Cancel (or a dialog/save failure) aborts.
func (a *App) ResolveUnsavedChanges() bool {
	if !a.activeDocument().Dirty {
		return true
	}
	return !a.promptUnsaved()
}

// beforeClose implements the unsaved-changes guard: Save / Don't Save /
// Cancel, matching the spec's error-handling contract.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	unsynced := a.session.HasUnsyncedDirty()
	if len(a.dirtyDocuments()) == 0 && !unsynced {
		return false
	}
	return a.promptUnsaved()
}

// promptUnsaved shows the Save / Don't Save / Cancel dialog and returns
// whether the pending action (close, open, …) must be prevented.
func (a *App) promptUnsaved() (prevent bool) {
	choice, err := a.native.ConfirmUnsaved(a.ctx)
	if err != nil {
		return true // dialog failed — do not lose data
	}
	switch choice {
	case "Don't Save":
		return false
	case "Save":
		return !a.saveCurrent()
	default: // Cancel
		return true
	}
}

// saveCurrent writes the latest known content. Returns false if the save
// failed or the user canceled a Save As dialog.
// saveCurrent saves EVERY tab with unsaved changes, each to its own path.
//
// It does not consult a "current" path. Choosing the target from ambient state
// is precisely what allowed one tab's content to be written over another tab's
// file. A pathless tab goes through Save As. Any failure or cancellation stops
// the close so the remaining documents are not lost.
func (a *App) saveCurrent() bool {
	a.mu.Lock()
	content := a.currentText
	a.mu.Unlock()
	unsynced := a.session.HasUnsyncedDirty()
	if unsynced {
		// No document list, so no known target. Ask rather than guess.
		path, err := a.SaveDocumentAs(content)
		return err == nil && path != ""
	}
	for _, doc := range a.dirtyDocuments() {
		if doc.Path == "" {
			path, err := a.SaveDocumentAs(doc.Content)
			if err != nil || path == "" {
				return false
			}
			continue
		}
		if err := a.SaveDocument(doc.Path, doc.Content); err != nil {
			return false
		}
	}
	return true
}

func (a *App) openPath(path string) (OpenResult, error) {
	content, err := a.documents.ReadMarkdown(path)
	if err != nil {
		a.events.Record("document.open.failed", map[string]string{"path": path, "error": err.Error()})
		a.native.ShowError(a.ctx, "Open Failed", err.Error())
		return OpenResult{}, err
	}
	a.events.Record("document.opened", map[string]string{"path": path, "bytes": strconv.Itoa(len(content))})
	a.mu.Lock()
	a.currentPath = path
	a.currentText = content
	a.rememberOnDisk(path, content)
	a.mu.Unlock()
	a.session.AdoptPath(path, content)
	a.recordRecent(path)
	a.updateTitle()
	return OpenResult{Path: path, Content: content}, nil
}

func (a *App) recordRecent(path string) {
	if a.preferences == nil || path == "" {
		return
	}
	_, _ = a.preferences.RecordRecent(path)
}

func (a *App) updateTitle() {
	if a.ctx == nil {
		return
	}
	active := a.activeDocument()
	path, dirty := active.Path, active.Dirty
	if path == "" {
		a.mu.Lock()
		path = a.currentPath
		a.mu.Unlock()
	}
	name := "untitled"
	if path != "" {
		name = path
	}
	title := "Dr. Markdown — " + name
	if dirty {
		title += " •"
	}
	a.native.SetTitle(a.ctx, title)
}

type wailsNative struct{}

func (wailsNative) OpenMarkdownFile(ctx context.Context) (string, error) {
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: "Open Markdown Document",
		Filters: []runtime.FileFilter{
			{DisplayName: "Markdown", Pattern: "*.md;*.markdown"},
			{DisplayName: "All Files", Pattern: "*"},
		},
	})
}

func (wailsNative) SaveMarkdownFile(ctx context.Context, defaultFilename string) (string, error) {
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:           "Save Markdown Document",
		DefaultFilename: defaultFilename,
		Filters:         []runtime.FileFilter{{DisplayName: "Markdown", Pattern: "*.md"}},
	})
}

func (wailsNative) SelectImageFile(ctx context.Context) (string, error) {
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: "Import Image",
		Filters: []runtime.FileFilter{
			{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg"},
			{DisplayName: "All Files", Pattern: "*"},
		},
	})
}

// SubscribeFileDrop forwards Wails file-drop paths to the frontend, which
// filters them to importable images.
func (wailsNative) SubscribeFileDrop(ctx context.Context) {
	runtime.OnFileDrop(ctx, func(_, _ int, paths []string) {
		runtime.EventsEmit(ctx, "files:dropped", paths)
	})
}

// RevealPath shows the file in the platform's file browser rather than opening
// it, so the user lands on the asset inside its document asset folder.
//
// This ships as one binary for macOS, Windows and Linux, so the command is
// selected from GOOS rather than assumed. It previously always ran macOS's
// `open -R`, which fails silently everywhere else.
func (wailsNative) RevealPath(_ context.Context, path string) error {
	name, args := revealCommand(runtime2.GOOS, path)
	if name == "" {
		return fmt.Errorf("reveal: unsupported platform %s", runtime2.GOOS)
	}
	return exec.Command(name, args...).Run()
}

// revealCommand maps an OS to its reveal-in-file-browser invocation. Split out
// from RevealPath so the mapping is testable without executing anything.
func revealCommand(goos, path string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}
	case "windows":
		// /select, must be one argument with the path appended.
		return "explorer", []string{"/select," + path}
	case "linux":
		// No universal "select the file" verb; open the containing directory.
		return "xdg-open", []string{filepath.Dir(path)}
	default:
		return "", nil
	}
}

func (wailsNative) ConfirmOverwriteChanged(ctx context.Context, path string) (string, error) {
	return runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "File Changed on Disk",
		Message:       filepath.Base(path) + " has been modified by another program since you opened it.\n\nSaving now replaces those changes with your version.",
		Buttons:       []string{"Overwrite", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
		Icon:          iconFreeMessageDialogPNG,
	})
}

func (wailsNative) OpenExternalURL(ctx context.Context, url string) error {
	runtime.BrowserOpenURL(ctx, url)
	return nil
}

func (wailsNative) ShowError(ctx context.Context, title string, message string) {
	runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.ErrorDialog,
		Title:   title,
		Message: message,
		Icon:    iconFreeMessageDialogPNG,
	})
}

func (wailsNative) ConfirmUnsaved(ctx context.Context) (string, error) {
	return runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Unsaved Changes",
		Message:       "Save changes before closing?",
		Buttons:       []string{"Save", "Don't Save", "Cancel"},
		DefaultButton: "Save",
		CancelButton:  "Cancel",
		Icon:          iconFreeMessageDialogPNG,
	})
}

func (wailsNative) SetTitle(ctx context.Context, title string) {
	runtime.WindowSetTitle(ctx, title)
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
