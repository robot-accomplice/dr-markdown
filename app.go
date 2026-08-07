package main

import (
	"context"
	"fmt"
	"os"
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

	mu          sync.Mutex
	dirty       bool
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

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

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
	a.dirty = false
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

// SetDirty records the frontend's dirty state and refreshes the title.
func (a *App) SetDirty(dirty bool) {
	a.mu.Lock()
	a.dirty = dirty
	a.mu.Unlock()
	a.updateTitle()
}

// UpdateContent stores the latest markdown (pushed debounced by the
// frontend) so the close guard can save without a round-trip.
func (a *App) UpdateContent(content string) {
	a.mu.Lock()
	a.currentText = content
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

// ImportImage selects an image, copies it into the document asset folder, and
// returns markdown for insertion. A canceled picker returns an empty result.
func (a *App) ImportImage(documentPath string) (imageassets.ImportedImage, error) {
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

// ResolveUnsavedChanges reports whether the frontend may discard the
// current dirty buffer (e.g. to open another document). Not dirty: true
// immediately. Otherwise it shows the same Save / Don't Save / Cancel
// dialog as the close guard; Save saves first, Don't Save proceeds, and
// Cancel (or a dialog/save failure) aborts.
func (a *App) ResolveUnsavedChanges() bool {
	a.mu.Lock()
	dirty := a.dirty
	a.mu.Unlock()
	if !dirty {
		return true
	}
	return !a.promptUnsaved()
}

// beforeClose implements the unsaved-changes guard: Save / Don't Save /
// Cancel, matching the spec's error-handling contract.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	a.mu.Lock()
	dirty := a.dirty
	a.mu.Unlock()
	if !dirty {
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
func (a *App) saveCurrent() bool {
	a.mu.Lock()
	content, path := a.currentText, a.currentPath
	a.mu.Unlock()
	if path == "" {
		p, err := a.SaveDocumentAs(content)
		return err == nil && p != ""
	}
	return a.SaveDocument(path, content) == nil
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
	a.dirty = false
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
	a.mu.Lock()
	path, dirty := a.currentPath, a.dirty
	a.mu.Unlock()
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
