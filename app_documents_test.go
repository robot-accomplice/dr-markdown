package main

import (
	"context"
	"testing"
)

// The close guard must save each document to ITS OWN path. Go previously held
// one ambient (currentPath, currentText): opening A and then typing in a new
// tab left the path pointing at A while the content came from B, so quitting
// wrote B's text over A. Reproduced before the fix; this pins it shut.
func TestCloseGuardNeverWritesOneDocumentOverAnother(t *testing.T) {
	native := &fakeNative{unsavedChoice: "Save", savePath: "/tmp/scratch.md"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/notes.md": "# Original notes\n"}}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
	app.startup(context.Background())

	native.openPath = "/tmp/notes.md"
	if _, err := app.OpenDocument(); err != nil {
		t.Fatalf("open: %v", err)
	}

	// The user hits Cmd-N and types. The frontend reports both tabs.
	app.SyncDocuments([]OpenDocument{
		{Path: "/tmp/notes.md", Content: "# Original notes\n", Dirty: false},
		{Path: "", Content: "scratch text from a different tab", Dirty: true, Active: true},
	})

	app.beforeClose(context.Background())

	if got := documents.writes["/tmp/notes.md"]; got != "" {
		t.Fatalf("the untouched document was written to: %q", got)
	}
	if got := documents.writes["/tmp/scratch.md"]; got != "scratch text from a different tab" {
		t.Fatalf("the dirty tab was not saved to its own path: %q", got)
	}
}

// Dirty state must aggregate across tabs. It previously tracked the ACTIVE
// document only, so quitting with a clean tab in front discarded every edited
// background tab with no prompt at all.
func TestCloseGuardPromptsForDirtyBackgroundTabs(t *testing.T) {
	native := &fakeNative{unsavedChoice: "Save"}
	documents := &fakeDocuments{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
	app.startup(context.Background())

	app.SyncDocuments([]OpenDocument{
		{Path: "/tmp/front.md", Content: "clean\n", Dirty: false, Active: true},
		{Path: "/tmp/background-a.md", Content: "edited a\n", Dirty: true},
		{Path: "/tmp/background-b.md", Content: "edited b\n", Dirty: true},
	})

	// Choosing Save and succeeding means the window may close — what must not
	// happen is closing WITHOUT the background tabs being written.
	if prevented := app.beforeClose(context.Background()); prevented {
		t.Fatal("close should proceed once every dirty tab saved successfully")
	}
	if documents.writes["/tmp/background-a.md"] != "edited a\n" {
		t.Errorf("background tab a was not saved: %#v", documents.writes)
	}
	if documents.writes["/tmp/background-b.md"] != "edited b\n" {
		t.Errorf("background tab b was not saved: %#v", documents.writes)
	}
}

// The prompt must actually fire for a dirty background tab: with a clean tab
// in front, dirty state used to read as false and the app quit silently.
func TestCloseGuardCancelKeepsDirtyBackgroundTabsOpen(t *testing.T) {
	native := &fakeNative{unsavedChoice: "Cancel"}
	documents := &fakeDocuments{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
	app.startup(context.Background())
	app.SyncDocuments([]OpenDocument{
		{Path: "/tmp/front.md", Content: "clean\n", Active: true},
		{Path: "/tmp/background.md", Content: "edited\n", Dirty: true},
	})

	if prevented := app.beforeClose(context.Background()); !prevented {
		t.Fatal("a dirty background tab must block the close when the user cancels")
	}
	if len(documents.writes) != 0 {
		t.Errorf("cancel must not write anything: %#v", documents.writes)
	}
}

// Every tab clean must still close without a prompt.
func TestCloseGuardAllowsCloseWhenNoTabIsDirty(t *testing.T) {
	app := newAppWithDependencies(appDependencies{
		native:      &fakeNative{},
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
	app.startup(context.Background())
	app.SyncDocuments([]OpenDocument{
		{Path: "/tmp/a.md", Content: "a\n", Active: true},
		{Path: "/tmp/b.md", Content: "b\n"},
	})

	if prevented := app.beforeClose(context.Background()); prevented {
		t.Fatal("a fully clean workspace should close without prompting")
	}
}

// One binary, three platforms: the reveal command must not be macOS's `open`
// everywhere. It was, so revealing an asset failed silently off macOS.
func TestRevealCommandIsSelectedPerPlatform(t *testing.T) {
	for _, tc := range []struct {
		goos string
		want string
	}{
		{"darwin", "open"},
		{"windows", "explorer"},
		{"linux", "xdg-open"},
	} {
		name, args := revealCommand(tc.goos, "/tmp/notes.assets/photo.png")
		if name != tc.want {
			t.Errorf("%s: command = %q, want %q", tc.goos, name, tc.want)
		}
		if len(args) == 0 {
			t.Errorf("%s: no arguments passed", tc.goos)
		}
	}
	if name, _ := revealCommand("plan9", "/x"); name != "" {
		t.Errorf("an unknown platform should report unsupported, got %q", name)
	}
}
