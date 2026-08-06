package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dr-markdown/internal/document"
)

// App is the Wails-bound API exposed to the frontend as window.go.main.App.
type App struct {
	ctx context.Context

	mu          sync.Mutex
	dirty       bool
	currentPath string
	currentText string
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// OpenResult is returned to the frontend when a document is opened.
type OpenResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// OpenDocument shows a native open dialog and reads the chosen file.
// A canceled dialog returns an empty OpenResult and nil error.
func (a *App) OpenDocument() (OpenResult, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open Markdown Document",
		Filters: []runtime.FileFilter{
			{DisplayName: "Markdown", Pattern: "*.md;*.markdown"},
			{DisplayName: "All Files", Pattern: "*"},
		},
	})
	if err != nil {
		return OpenResult{}, err
	}
	if path == "" {
		return OpenResult{}, nil
	}
	content, err := document.Read(path)
	if err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Open Failed",
			Message: err.Error(),
		})
		return OpenResult{}, err
	}
	a.mu.Lock()
	a.currentPath = path
	a.currentText = content
	a.dirty = false
	a.mu.Unlock()
	a.updateTitle()
	return OpenResult{Path: path, Content: content}, nil
}

// SaveDocument writes content to path atomically. path must be non-empty.
func (a *App) SaveDocument(path, content string) error {
	if path == "" {
		return fmt.Errorf("SaveDocument: empty path")
	}
	if err := document.WriteAtomic(path, content); err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Save Failed",
			Message: err.Error(),
		})
		return err
	}
	a.mu.Lock()
	a.currentPath = path
	a.currentText = content
	a.dirty = false
	a.mu.Unlock()
	a.updateTitle()
	return nil
}

// SaveDocumentAs shows a native save dialog and writes content atomically.
// Returns the saved path, or "" if the user canceled.
func (a *App) SaveDocumentAs(content string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save Markdown Document",
		DefaultFilename: "untitled.md",
		Filters:         []runtime.FileFilter{{DisplayName: "Markdown", Pattern: "*.md"}},
	})
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
	choice, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Unsaved Changes",
		Message:       "Save changes before closing?",
		Buttons:       []string{"Save", "Don't Save", "Cancel"},
		DefaultButton: "Save",
		CancelButton:  "Cancel",
	})
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
	runtime.WindowSetTitle(a.ctx, title)
}
