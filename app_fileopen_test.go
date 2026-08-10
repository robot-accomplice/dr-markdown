package main

import (
	"context"
	"testing"
)

// macOS hands a double-clicked file to the app through an open-documents event,
// not as a command-line argument. The bundle has always advertised the
// association (CFBundleDocumentTypes), but nothing consumed the event, so the
// file was silently dropped and the user got an empty document (#53).
//
// The event can arrive BEFORE the webview can accept a document — that is the
// normal case when the app is launched by double-clicking — so it has to be
// held until the frontend says it is listening.

func fileOpenApp(native *fakeNative, documents *fakeDocuments) *App {
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
	app.startup(context.Background())
	return app
}

// The launch case: the file arrives first, so it must be held rather than
// emitted into a webview that cannot yet receive it.
func TestFileOpenedBeforeTheFrontendIsReadyIsHeldAndThenDelivered(t *testing.T) {
	native := &fakeNative{}
	app := fileOpenApp(native, &fakeDocuments{})

	app.openFileFromOS("/tmp/notes.md")

	if len(native.emittedOpens) != 0 {
		t.Errorf("emitted %v into a frontend that is not listening", native.emittedOpens)
	}
	pending := app.FrontendReady()
	if len(pending) != 1 || pending[0] != "/tmp/notes.md" {
		t.Fatalf("FrontendReady() = %v, want [/tmp/notes.md]", pending)
	}
}

// A second call must not replay. The frontend calls this on every boot, and a
// reload must not reopen a document the user already closed.
func TestPendingFilesAreDeliveredOnlyOnce(t *testing.T) {
	native := &fakeNative{}
	app := fileOpenApp(native, &fakeDocuments{})

	app.openFileFromOS("/tmp/notes.md")
	app.FrontendReady()

	if again := app.FrontendReady(); len(again) != 0 {
		t.Errorf("second FrontendReady() replayed %v", again)
	}
}

// The already-running case: the app is open and the user double-clicks another
// file. There is nothing to wait for, so it goes straight to the frontend.
func TestFileOpenedWhileRunningIsEmittedImmediately(t *testing.T) {
	native := &fakeNative{}
	app := fileOpenApp(native, &fakeDocuments{})
	app.FrontendReady()

	app.openFileFromOS("/tmp/second.md")

	if len(native.emittedOpens) != 1 || native.emittedOpens[0] != "/tmp/second.md" {
		t.Errorf("emitted %v, want [/tmp/second.md]", native.emittedOpens)
	}
	if held := app.FrontendReady(); len(held) != 0 {
		t.Errorf("it was also held: %v — the user would get the document twice", held)
	}
}

// macOS is not the only caller, and an empty path names no document.
func TestAnEmptyPathIsIgnored(t *testing.T) {
	native := &fakeNative{}
	app := fileOpenApp(native, &fakeDocuments{})
	app.FrontendReady()

	app.openFileFromOS("")

	if len(native.emittedOpens) != 0 {
		t.Errorf("emitted %v for an empty path", native.emittedOpens)
	}
}

// Several files can arrive before the frontend is ready — selecting three in
// Finder and pressing Enter sends three events — and all must survive in order.
func TestEveryHeldFileSurvivesInOrder(t *testing.T) {
	native := &fakeNative{}
	app := fileOpenApp(native, &fakeDocuments{})

	app.openFileFromOS("/tmp/a.md")
	app.openFileFromOS("/tmp/b.md")

	pending := app.FrontendReady()
	if len(pending) != 2 || pending[0] != "/tmp/a.md" || pending[1] != "/tmp/b.md" {
		t.Errorf("FrontendReady() = %v, want [/tmp/a.md /tmp/b.md]", pending)
	}
}
