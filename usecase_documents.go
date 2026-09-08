package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	"dr-markdown/internal/eventlog"
	"dr-markdown/internal/session"
)

// documentUseCase owns open, save and resolve-unsaved. It holds the
// currentPath/currentText state those operations mutate — never a write
// target inferred from ambient state — and renders the window title, which
// is a function of exactly that state plus the session.
type documentUseCase struct {
	native      nativePort
	documents   documentPort
	preferences preferencePort
	events      *eventlog.Log
	session     *session.Session

	mu          sync.Mutex
	currentPath string
	currentText string
}

func (uc *documentUseCase) openViaDialog(ctx context.Context) (OpenResult, error) {
	path, err := uc.native.OpenMarkdownFile(ctx)
	if err != nil {
		return OpenResult{}, err
	}
	if path == "" {
		return OpenResult{}, nil
	}
	return uc.open(ctx, path)
}

// save writes content to path atomically. path must be non-empty.
//
// Before overwriting, it checks that the file on disk still holds the bytes the
// app last saw there. Nothing did that before, so a change made by anything else
// — a git pull, a sync client, a second window — was replaced with no error and
// no prompt.
func (uc *documentUseCase) save(ctx context.Context, path, content string) error {
	if path == "" {
		return fmt.Errorf("SaveDocument: empty path")
	}
	if err := uc.confirmNoExternalChange(ctx, path); err != nil {
		return err
	}
	if err := uc.documents.WriteMarkdown(path, content); err != nil {
		uc.events.Record("document.save.failed", map[string]string{"path": path, "error": err.Error()})
		uc.native.ShowError(ctx, "Save Failed", err.Error())
		return err
	}
	uc.events.Record("document.saved", map[string]string{"path": path, "bytes": strconv.Itoa(len(content))})
	uc.mu.Lock()
	uc.currentPath = path
	uc.currentText = content
	uc.mu.Unlock()
	uc.session.RememberOnDisk(path, content)
	uc.session.AdoptPath(path, content)
	uc.recordRecent(path)
	uc.updateTitle(ctx)
	return nil
}

// saveAs shows a native save dialog and writes content atomically.
// Returns the saved path, or "" if the user canceled.
func (uc *documentUseCase) saveAs(ctx context.Context, content string) (string, error) {
	path, err := uc.native.SaveMarkdownFile(ctx, "untitled.md")
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	if err := uc.save(ctx, path, content); err != nil {
		return "", err
	}
	return path, nil
}

func (uc *documentUseCase) open(ctx context.Context, path string) (OpenResult, error) {
	content, err := uc.documents.ReadMarkdown(path)
	if err != nil {
		uc.events.Record("document.open.failed", map[string]string{"path": path, "error": err.Error()})
		uc.native.ShowError(ctx, "Open Failed", err.Error())
		return OpenResult{}, err
	}
	uc.events.Record("document.opened", map[string]string{"path": path, "bytes": strconv.Itoa(len(content))})
	uc.mu.Lock()
	uc.currentPath = path
	uc.currentText = content
	uc.mu.Unlock()
	uc.session.RememberOnDisk(path, content)
	uc.session.AdoptPath(path, content)
	uc.recordRecent(path)
	uc.updateTitle(ctx)
	return OpenResult{Path: path, Content: content}, nil
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
func (uc *documentUseCase) confirmNoExternalChange(ctx context.Context, path string) error {
	expected, known := uc.session.BaselineFor(path)
	if !known {
		return nil
	}
	current, err := uc.documents.ReadMarkdown(path)
	if err != nil || current == expected {
		return nil
	}
	choice, err := uc.native.ConfirmOverwriteChanged(ctx, path)
	if err != nil {
		return err
	}
	uc.events.Record("document.conflict", map[string]string{"path": path, "choice": choice})
	if choice != "Overwrite" {
		return fmt.Errorf("save canceled: %s changed on disk since it was opened", filepath.Base(path))
	}
	return nil
}

// resolveUnsaved reports whether the frontend may discard the
// current dirty buffer (e.g. to open another document). Not dirty: true
// immediately. Otherwise it shows the same Save / Don't Save / Cancel
// dialog as the close guard; Save saves first, Don't Save proceeds, and
// Cancel (or a dialog/save failure) aborts.
func (uc *documentUseCase) resolveUnsaved(ctx context.Context) bool {
	if !uc.session.Active().Dirty {
		return true
	}
	return !uc.promptUnsaved(ctx)
}

// preventClose implements the unsaved-changes guard: Save / Don't Save /
// Cancel, matching the spec's error-handling contract.
func (uc *documentUseCase) preventClose(ctx context.Context) (prevent bool) {
	unsynced := uc.session.HasUnsyncedDirty()
	if len(uc.session.Dirty()) == 0 && !unsynced {
		return false
	}
	return uc.promptUnsaved(ctx)
}

// promptUnsaved shows the Save / Don't Save / Cancel dialog and returns
// whether the pending action (close, open, …) must be prevented.
func (uc *documentUseCase) promptUnsaved(ctx context.Context) (prevent bool) {
	choice, err := uc.native.ConfirmUnsaved(ctx)
	if err != nil {
		return true // dialog failed — do not lose data
	}
	switch choice {
	case "Don't Save":
		return false
	case "Save":
		return !uc.saveCurrent(ctx)
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
func (uc *documentUseCase) saveCurrent(ctx context.Context) bool {
	uc.mu.Lock()
	content := uc.currentText
	uc.mu.Unlock()
	unsynced := uc.session.HasUnsyncedDirty()
	if unsynced {
		// No document list, so no known target. Ask rather than guess.
		path, err := uc.saveAs(ctx, content)
		return err == nil && path != ""
	}
	for _, doc := range uc.session.Dirty() {
		if doc.Path == "" {
			path, err := uc.saveAs(ctx, doc.Content)
			if err != nil || path == "" {
				return false
			}
			continue
		}
		if err := uc.save(ctx, doc.Path, doc.Content); err != nil {
			return false
		}
	}
	return true
}

func (uc *documentUseCase) recordRecent(path string) {
	if uc.preferences == nil || path == "" {
		return
	}
	_, _ = uc.preferences.RecordRecent(path)
}

func (uc *documentUseCase) updateTitle(ctx context.Context) {
	if ctx == nil {
		return
	}
	active := uc.session.Active()
	path, dirty := active.Path, active.Dirty
	if path == "" {
		uc.mu.Lock()
		path = uc.currentPath
		uc.mu.Unlock()
	}
	name := "untitled"
	if path != "" {
		name = path
	}
	title := "Dr Markdown — " + name
	if dirty {
		title += " •"
	}
	uc.native.SetTitle(ctx, title)
}
