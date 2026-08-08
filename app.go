package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	imageassets "dr-markdown/internal/assets"
	"dr-markdown/internal/document"
	"dr-markdown/internal/fonts"
	"dr-markdown/internal/preferences"
)

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

	mu   sync.Mutex
	docs []OpenDocument
	// currentPath is the last path opened or saved. It names the window title
	// and the Save-As default only; it is NEVER a write target, because
	// inferring the target from ambient state is what destroyed documents.
	currentPath string
	currentText string
	native      nativePort
	documents   documentPort
	fonts       fontPort
	preferences preferencePort
	images      imageAssetPort
}

type appDependencies struct {
	native      nativePort
	documents   documentPort
	fonts       fontPort
	preferences preferencePort
	images      imageAssetPort
}

type nativePort interface {
	OpenMarkdownFile(context.Context) (string, error)
	SaveMarkdownFile(context.Context, string) (string, error)
	SelectImageFile(context.Context) (string, error)
	RevealPath(context.Context, string) error
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
	return newAppWithDependencies(appDependencies{
		native:      wailsNative{},
		documents:   documentAdapter{},
		fonts:       fontAdapter{},
		preferences: store,
		images:      imageAssetAdapter{},
	})
}

func newAppWithDependencies(deps appDependencies) *App {
	return &App{
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
func (a *App) SaveDocument(path, content string) error {
	if path == "" {
		return fmt.Errorf("SaveDocument: empty path")
	}
	if err := a.documents.WriteMarkdown(path, content); err != nil {
		a.native.ShowError(a.ctx, "Save Failed", err.Error())
		return err
	}
	a.mu.Lock()
	a.currentPath = path
	a.currentText = content
	for i := range a.docs {
		if a.docs[i].Path == path || (a.docs[i].Active && a.docs[i].Path == "") {
			a.docs[i].Path = path
			a.docs[i].Content = content
			a.docs[i].Dirty = false
		}
	}
	a.mu.Unlock()
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
type OpenDocument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Dirty   bool   `json:"dirty"`
	Active  bool   `json:"active"`
}

// SyncDocuments replaces Go's view of the open tabs.
//
// This exists because Go used to hold a single ambient (currentPath,
// currentText). The frontend is multi-tab and never reset the path when a new
// tab opened, so the close guard wrote the new tab's text over the previously
// opened FILE — silently destroying a document the user had not touched. Any
// write must now name its own target, and dirty state aggregates across tabs
// rather than tracking only the visible one.
func (a *App) SyncDocuments(docs []OpenDocument) {
	a.mu.Lock()
	a.docs = append(a.docs[:0], docs...)
	a.mu.Unlock()
	a.updateTitle()
}

// SetDirty records the frontend's dirty state for the active tab.
func (a *App) SetDirty(dirty bool) {
	a.mu.Lock()
	a.ensureImplicitDocument()
	for i := range a.docs {
		if a.docs[i].Active {
			a.docs[i].Dirty = dirty
		}
	}
	a.mu.Unlock()
	a.updateTitle()
}

// dirtyDocuments returns a copy of every tab with unsaved changes.
func (a *App) dirtyDocuments() []OpenDocument {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []OpenDocument
	for _, d := range a.docs {
		if d.Dirty {
			out = append(out, d)
		}
	}
	return out
}

// activeDocument returns the focused tab, or the first, or a zero value.
func (a *App) activeDocument() OpenDocument {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range a.docs {
		if d.Active {
			return d
		}
	}
	if len(a.docs) > 0 {
		return a.docs[0]
	}
	return OpenDocument{}
}

// UpdateContent stores the latest markdown for the active tab (pushed
// debounced by the frontend) so the close guard can save without a round-trip.
func (a *App) UpdateContent(content string) {
	a.mu.Lock()
	a.ensureImplicitDocument()
	for i := range a.docs {
		if a.docs[i].Active {
			a.docs[i].Content = content
		}
	}
	a.mu.Unlock()
}

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
	if len(a.dirtyDocuments()) == 0 {
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

// ensureImplicitDocument keeps single-document callers working: when nothing
// has been synced yet there is exactly one buffer, so deriving it from the
// last opened path is unambiguous rather than a guess between tabs.
// Callers must hold a.mu.
func (a *App) ensureImplicitDocument() {
	if len(a.docs) == 0 {
		a.docs = append(a.docs, OpenDocument{Path: a.currentPath, Content: a.currentText, Active: true})
	}
}

func (a *App) openPath(path string) (OpenResult, error) {
	content, err := a.documents.ReadMarkdown(path)
	if err != nil {
		a.native.ShowError(a.ctx, "Open Failed", err.Error())
		return OpenResult{}, err
	}
	a.mu.Lock()
	a.currentPath = path
	a.currentText = content
	for i := range a.docs {
		if a.docs[i].Path == path || (a.docs[i].Active && a.docs[i].Path == "") {
			a.docs[i].Path = path
			a.docs[i].Content = content
			a.docs[i].Dirty = false
		}
	}
	a.mu.Unlock()
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

// RevealPath selects the file in Finder rather than opening it, so the user
// lands on the asset inside its document asset folder.
func (wailsNative) RevealPath(_ context.Context, path string) error {
	return exec.Command("open", "-R", path).Run()
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
