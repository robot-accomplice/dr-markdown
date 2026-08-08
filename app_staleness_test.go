package main

import (
	"context"
	"testing"
)

// Nothing compared the file on disk against what the app last read before
// overwriting it, so a change made by anything else — a git pull, a sync
// client, a second window, an editor in another app — was replaced silently
// with no error and no prompt. This was raised as a blocking finding in ABORT
// Round 1 and carried across three rounds without ever being justified.
//
// The rule: never overwrite bytes the app has not seen. Detect, then ask.
func TestSaveRefusesToClobberAChangeMadeOnDisk(t *testing.T) {
	native := &fakeNative{openPath: "/tmp/notes.md", overwriteChoice: "Cancel"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/notes.md": "# Original\n"}}
	app := newAppWithDependencies(appDependencies{
		native: native, documents: documents, fonts: fakeFonts{}, preferences: &fakePreferences{},
	})
	app.startup(context.Background())

	if _, err := app.OpenDocument(); err != nil {
		t.Fatalf("open: %v", err)
	}

	// Something else writes the file while it is open.
	documents.read["/tmp/notes.md"] = "# Edited by something else\n"

	err := app.SaveDocument("/tmp/notes.md", "# My version\n")
	if err == nil {
		t.Fatal("saving over an externally modified file must not silently succeed")
	}
	if got := documents.writes["/tmp/notes.md"]; got != "" {
		t.Errorf("the external change was clobbered anyway: %q", got)
	}
	if !native.overwriteAsked {
		t.Error("the user was never asked")
	}
}

// Detection must not become a dead end. The user is asked, and choosing to
// overwrite writes their version.
func TestSaveProceedsWhenTheUserChoosesToOverwrite(t *testing.T) {
	native := &fakeNative{openPath: "/tmp/notes.md", overwriteChoice: "Overwrite"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/notes.md": "# Original\n"}}
	app := newAppWithDependencies(appDependencies{
		native: native, documents: documents, fonts: fakeFonts{}, preferences: &fakePreferences{},
	})
	app.startup(context.Background())
	if _, err := app.OpenDocument(); err != nil {
		t.Fatalf("open: %v", err)
	}
	documents.read["/tmp/notes.md"] = "# Edited elsewhere\n"

	if err := app.SaveDocument("/tmp/notes.md", "# My version\n"); err != nil {
		t.Fatalf("overwrite was chosen and must succeed: %v", err)
	}
	if got := documents.writes["/tmp/notes.md"]; got != "# My version\n" {
		t.Errorf("chosen overwrite did not write: %q", got)
	}
}

// The check must not fire on ordinary saves, or it teaches users to click
// through it — which is worse than not having it. An unchanged file, a
// repeated save, and a file the app has never read all save without a prompt.
func TestOrdinarySavesAreNeverInterrupted(t *testing.T) {
	native := &fakeNative{openPath: "/tmp/notes.md"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/notes.md": "# Original\n"}}
	app := newAppWithDependencies(appDependencies{
		native: native, documents: documents, fonts: fakeFonts{}, preferences: &fakePreferences{},
	})
	app.startup(context.Background())
	if _, err := app.OpenDocument(); err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := app.SaveDocument("/tmp/notes.md", "# First edit\n"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// Saving again must compare against what WE just wrote, not against the
	// content read at open — otherwise the second save always looks stale.
	if err := app.SaveDocument("/tmp/notes.md", "# Second edit\n"); err != nil {
		t.Fatalf("second save: %v", err)
	}
	// A path the app has never read has nothing to compare against.
	if err := app.SaveDocument("/tmp/brand-new.md", "# New\n"); err != nil {
		t.Fatalf("new file: %v", err)
	}
	if native.overwriteAsked {
		t.Error("the user was prompted during ordinary saves")
	}
	if documents.writes["/tmp/notes.md"] != "# Second edit\n" {
		t.Errorf("second save did not land: %q", documents.writes["/tmp/notes.md"])
	}
}
