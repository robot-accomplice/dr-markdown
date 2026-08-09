package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The write-staleness control refuses to overwrite a file that changed on disk
// since the app last read or wrote it.
//
// It shipped with no test at all, which is this project's most expensive
// recurring shape: ABORT Round 4 named "control not exercised" as a root-cause
// cluster, and a control nobody exercises is indistinguishable from one that
// does not work. The scenario it exists for — a git pull, a sync client, or a
// second window touching the file while it is open — destroys work silently and
// is not reproducible after the fact.
//
// Each test below states the property, not the implementation, so replacing
// content comparison with mtime or a hash does not invalidate them.

func stalenessApp(native *fakeNative, documents *fakeDocuments) *App {
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
	app.startup(context.Background())
	return app
}

// The core promise: work changed outside the app is not destroyed by a save.
func TestSaveRefusesToOverwriteAFileChangedOnDisk(t *testing.T) {
	native := &fakeNative{overwriteChoice: "Cancel"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/notes.md": "# Original\n"}}
	app := stalenessApp(native, documents)

	// Open establishes the baseline the check compares against.
	if _, err := app.OpenRecentDocument("/tmp/notes.md"); err != nil {
		t.Fatalf("OpenRecentDocument: %v", err)
	}

	// Something else edits the file — a git pull, a sync client, another window.
	documents.read["/tmp/notes.md"] = "# Changed by someone else\n"

	err := app.SaveDocument("/tmp/notes.md", "# My version\n")
	if err == nil {
		t.Fatal("save overwrote a file that changed on disk without refusing")
	}
	if !native.overwriteAsked {
		t.Error("the user was never asked; the conflict was resolved silently")
	}
	if got := documents.writes["/tmp/notes.md"]; got != "" {
		t.Errorf("a refused save still wrote to disk: %q", got)
	}
	if !strings.Contains(err.Error(), "changed on disk") {
		t.Errorf("refusal did not say why: %v", err)
	}
}

// Refusing must be overridable, or the user cannot save their own work.
func TestSaveProceedsWhenTheUserChoosesToOverwrite(t *testing.T) {
	native := &fakeNative{overwriteChoice: "Overwrite"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/notes.md": "# Original\n"}}
	app := stalenessApp(native, documents)

	if _, err := app.OpenRecentDocument("/tmp/notes.md"); err != nil {
		t.Fatalf("OpenRecentDocument: %v", err)
	}
	documents.read["/tmp/notes.md"] = "# Changed by someone else\n"

	if err := app.SaveDocument("/tmp/notes.md", "# My version\n"); err != nil {
		t.Fatalf("an explicitly confirmed overwrite was refused: %v", err)
	}
	if got := documents.writes["/tmp/notes.md"]; got != "# My version\n" {
		t.Errorf("confirmed overwrite did not write the content: %q", got)
	}
}

// A prompt that fires when nothing changed is worse than no prompt: the user
// learns to click through it, and then it protects nothing. Saving repeatedly
// to the same file must never ask.
func TestRepeatedSavesToTheSameFileNeverPrompt(t *testing.T) {
	native := &fakeNative{overwriteChoice: "Cancel"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/notes.md": "# Original\n"}}
	app := stalenessApp(native, documents)

	if _, err := app.OpenRecentDocument("/tmp/notes.md"); err != nil {
		t.Fatalf("OpenRecentDocument: %v", err)
	}
	for i, content := range []string{"# One\n", "# Two\n", "# Three\n"} {
		if err := app.SaveDocument("/tmp/notes.md", content); err != nil {
			t.Fatalf("save %d refused with no external change: %v", i+1, err)
		}
	}
	if native.overwriteAsked {
		t.Error("the app asked about an external change that never happened; " +
			"a prompt the user learns to dismiss protects nothing")
	}
}

// Failing to VERIFY is not evidence of a conflict. If the file cannot be
// re-read, blocking the save would strand the user's work over a check that
// could not run — the failure mode must not be "you cannot save".
func TestSaveIsNotBlockedWhenTheFileCannotBeReRead(t *testing.T) {
	native := &fakeNative{overwriteChoice: "Cancel"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/notes.md": "# Original\n"}}
	app := stalenessApp(native, documents)

	if _, err := app.OpenRecentDocument("/tmp/notes.md"); err != nil {
		t.Fatalf("OpenRecentDocument: %v", err)
	}
	documents.readErr = errors.New("permission denied")

	if err := app.SaveDocument("/tmp/notes.md", "# My version\n"); err != nil {
		t.Fatalf("an unverifiable file blocked the save: %v", err)
	}
	if got := documents.writes["/tmp/notes.md"]; got != "# My version\n" {
		t.Errorf("content was not written: %q", got)
	}
}

// A path the app has never read or written has no baseline, so there is nothing
// to compare and nothing to warn about.
func TestSavingToAnUntouchedPathDoesNotPrompt(t *testing.T) {
	native := &fakeNative{overwriteChoice: "Cancel"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/someone-elses.md": "# Not ours\n"}}
	app := stalenessApp(native, documents)

	if err := app.SaveDocument("/tmp/someone-elses.md", "# Ours now\n"); err != nil {
		t.Fatalf("save to an untouched path was refused: %v", err)
	}
	if native.overwriteAsked {
		t.Error("prompted about a path the app has no baseline for")
	}
}
