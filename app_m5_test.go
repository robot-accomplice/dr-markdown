package main

import (
	"context"
	"errors"
	"testing"

	"dr-markdown/internal/preferences"
)

func TestAppOpenDocumentRecordsRecentAndUpdatesTitle(t *testing.T) {
	native := &fakeNative{openPath: "/tmp/opened.md"}
	documents := &fakeDocuments{read: map[string]string{"/tmp/opened.md": "# Opened\n"}}
	prefs := &fakePreferences{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: prefs,
	})
	app.startup(context.Background())

	result, err := app.OpenDocument()
	if err != nil {
		t.Fatalf("OpenDocument returned error: %v", err)
	}
	if result.Path != "/tmp/opened.md" || result.Content != "# Opened\n" {
		t.Fatalf("OpenDocument result = %#v", result)
	}
	if len(prefs.recorded) != 1 || prefs.recorded[0] != "/tmp/opened.md" {
		t.Fatalf("recent file was not recorded: %#v", prefs.recorded)
	}
	if native.title != "Dr. Markdown — /tmp/opened.md" {
		t.Fatalf("title = %q", native.title)
	}
}

func TestAppSaveAsRoutesThroughDialogPersistsRecentAndClearsDirty(t *testing.T) {
	native := &fakeNative{savePath: "/tmp/saved.md"}
	documents := &fakeDocuments{}
	prefs := &fakePreferences{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: prefs,
	})
	app.startup(context.Background())
	app.UpdateContent("# Draft\n")
	app.SetDirty(true)

	path, err := app.SaveDocumentAs("# Draft\n")
	if err != nil {
		t.Fatalf("SaveDocumentAs returned error: %v", err)
	}
	if path != "/tmp/saved.md" {
		t.Fatalf("saved path = %q", path)
	}
	if documents.writes["/tmp/saved.md"] != "# Draft\n" {
		t.Fatalf("saved content = %q", documents.writes["/tmp/saved.md"])
	}
	if len(prefs.recorded) != 1 || prefs.recorded[0] != "/tmp/saved.md" {
		t.Fatalf("saved file was not recorded as recent: %#v", prefs.recorded)
	}
	if native.title != "Dr. Markdown — /tmp/saved.md" {
		t.Fatalf("title = %q", native.title)
	}
	if app.ResolveUnsavedChanges() != true {
		t.Fatal("saved app should no longer prompt as dirty")
	}
}

func TestAppResolveUnsavedChangesSavesCurrentDocumentWhenUserChoosesSave(t *testing.T) {
	native := &fakeNative{unsavedChoice: "Save"}
	documents := &fakeDocuments{}
	prefs := &fakePreferences{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: prefs,
	})
	app.startup(context.Background())
	if err := app.SaveDocument("/tmp/current.md", "# Saved\n"); err != nil {
		t.Fatalf("seed saved doc: %v", err)
	}
	// The frontend names the document; Go no longer infers it from the last
	// path it happened to touch.
	app.SyncDocuments([]OpenDocument{{Path: "/tmp/current.md", Content: "# Saved\n", Active: true}})
	app.UpdateContent("# Changed\n")
	app.SetDirty(true)

	if proceed := app.ResolveUnsavedChanges(); !proceed {
		t.Fatal("ResolveUnsavedChanges should proceed after successful save")
	}
	if documents.writes["/tmp/current.md"] != "# Changed\n" {
		t.Fatalf("current document was not saved before proceeding: %#v", documents.writes)
	}
}

func TestAppLoadAndSavePreferencesDelegateToStore(t *testing.T) {
	prefs := &fakePreferences{
		loaded: preferences.Preferences{
			Settings: map[string]any{"theme": "dark"},
		},
	}
	app := newAppWithDependencies(appDependencies{
		native:      &fakeNative{},
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: prefs,
	})

	loaded, err := app.LoadPreferences()
	if err != nil {
		t.Fatalf("LoadPreferences returned error: %v", err)
	}
	if loaded.Settings["theme"] != "dark" {
		t.Fatalf("loaded preferences = %#v", loaded)
	}

	next := preferences.Preferences{RawOptions: map[string]any{"lineNumbers": false}}
	if err := app.SavePreferences(next); err != nil {
		t.Fatalf("SavePreferences returned error: %v", err)
	}
	if prefs.saved.RawOptions["lineNumbers"] != false {
		t.Fatalf("saved preferences = %#v", prefs.saved)
	}
}

func TestAppOpenRecentDocumentReadsSpecificPathWithoutDialog(t *testing.T) {
	native := &fakeNative{}
	documents := &fakeDocuments{read: map[string]string{"/tmp/recent.md": "# Recent\n"}}
	prefs := &fakePreferences{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: prefs,
	})
	app.startup(context.Background())

	result, err := app.OpenRecentDocument("/tmp/recent.md")
	if err != nil {
		t.Fatalf("OpenRecentDocument returned error: %v", err)
	}
	if result.Content != "# Recent\n" || result.Path != "/tmp/recent.md" {
		t.Fatalf("OpenRecentDocument result = %#v", result)
	}
	if native.openCalled {
		t.Fatal("OpenRecentDocument should not show the native open dialog")
	}
	if len(prefs.recorded) != 1 || prefs.recorded[0] != "/tmp/recent.md" {
		t.Fatalf("recent path was not recorded: %#v", prefs.recorded)
	}
}

func TestAppOpenDocumentCancelDoesNotReadOrRecord(t *testing.T) {
	documents := &fakeDocuments{}
	prefs := &fakePreferences{}
	app := newAppWithDependencies(appDependencies{
		native:      &fakeNative{},
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: prefs,
	})

	result, err := app.OpenDocument()
	if err != nil {
		t.Fatalf("OpenDocument cancel returned error: %v", err)
	}
	if result != (OpenResult{}) {
		t.Fatalf("OpenDocument cancel result = %#v", result)
	}
	if documents.readCount != 0 || len(prefs.recorded) != 0 {
		t.Fatalf("cancel should not read or record recents; reads=%d recents=%v", documents.readCount, prefs.recorded)
	}
}

func TestAppOpenDocumentReadErrorShowsNativeError(t *testing.T) {
	native := &fakeNative{openPath: "/tmp/missing.md"}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{readErr: errors.New("missing")},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})

	if _, err := app.OpenDocument(); err == nil {
		t.Fatal("OpenDocument should return read errors")
	}
	if native.errorTitle != "Open Failed" || native.errorMessage != "missing" {
		t.Fatalf("native error = %q %q", native.errorTitle, native.errorMessage)
	}
}

func TestAppSaveDocumentRejectsEmptyPathAndReportsWriteError(t *testing.T) {
	native := &fakeNative{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{writeErr: errors.New("disk full")},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})

	if err := app.SaveDocument("", "x"); err == nil {
		t.Fatal("SaveDocument should reject an empty path")
	}
	if err := app.SaveDocument("/tmp/fail.md", "x"); err == nil {
		t.Fatal("SaveDocument should return write errors")
	}
	if native.errorTitle != "Save Failed" || native.errorMessage != "disk full" {
		t.Fatalf("native error = %q %q", native.errorTitle, native.errorMessage)
	}
}

func TestAppSaveDocumentAsCancelDoesNotWrite(t *testing.T) {
	documents := &fakeDocuments{}
	app := newAppWithDependencies(appDependencies{
		native:      &fakeNative{},
		documents:   documents,
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})

	path, err := app.SaveDocumentAs("# Draft\n")
	if err != nil {
		t.Fatalf("SaveDocumentAs cancel returned error: %v", err)
	}
	if path != "" {
		t.Fatalf("SaveDocumentAs cancel path = %q", path)
	}
	if len(documents.writes) != 0 {
		t.Fatalf("cancel should not write: %#v", documents.writes)
	}
}

func TestAppResolveUnsavedChangesHonorsDiscardCancelAndDialogError(t *testing.T) {
	tests := []struct {
		name   string
		choice string
		want   bool
	}{
		{name: "discard proceeds", choice: "Don't Save", want: true},
		{name: "cancel stops", choice: "Cancel", want: false},
		{name: "dialog error stops", choice: "error", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newAppWithDependencies(appDependencies{
				native:      &fakeNative{unsavedChoice: tt.choice},
				documents:   &fakeDocuments{},
				fonts:       fakeFonts{},
				preferences: &fakePreferences{},
			})
			app.SyncDocuments([]OpenDocument{{Path: "/tmp/current.md", Content: "x", Active: true}})
			app.SetDirty(true)
			if got := app.ResolveUnsavedChanges(); got != tt.want {
				t.Fatalf("ResolveUnsavedChanges = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppBeforeCloseUsesUnsavedGuard(t *testing.T) {
	app := newAppWithDependencies(appDependencies{
		native:      &fakeNative{unsavedChoice: "Cancel"},
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
	if prevent := app.beforeClose(context.Background()); prevent {
		t.Fatal("clean document should not prevent close")
	}
	app.SetDirty(true)
	if prevent := app.beforeClose(context.Background()); !prevent {
		t.Fatal("dirty document should prevent close when user cancels")
	}
}

func TestAppListFontFamiliesDelegatesToProvider(t *testing.T) {
	app := newAppWithDependencies(appDependencies{
		native:      &fakeNative{},
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{families: []string{"Fira Code", "Menlo"}},
		preferences: &fakePreferences{},
	})
	got := app.ListFontFamilies()
	if len(got) != 2 || got[0] != "Fira Code" || got[1] != "Menlo" {
		t.Fatalf("ListFontFamilies = %v", got)
	}
}

type fakeNative struct {
	openPath      string
	savePath      string
	imagePath     string
	unsavedChoice string
	title         string
	openCalled    bool
	imageCalled   bool
	revealedPath  string
	externalURL   string

	fileDropSubscribed bool
	errorTitle         string
	errorMessage       string
}

func (f *fakeNative) OpenMarkdownFile(context.Context) (string, error) {
	f.openCalled = true
	return f.openPath, nil
}

func (f *fakeNative) SaveMarkdownFile(context.Context, string) (string, error) {
	return f.savePath, nil
}

func (f *fakeNative) SelectImageFile(context.Context) (string, error) {
	f.imageCalled = true
	return f.imagePath, nil
}

func (f *fakeNative) SubscribeFileDrop(context.Context) { f.fileDropSubscribed = true }

func (f *fakeNative) RevealPath(_ context.Context, path string) error {
	f.revealedPath = path
	return nil
}

func (f *fakeNative) OpenExternalURL(_ context.Context, url string) error {
	f.externalURL = url
	return nil
}

func (f *fakeNative) ShowError(_ context.Context, title string, message string) {
	f.errorTitle = title
	f.errorMessage = message
}

func (f *fakeNative) ConfirmUnsaved(context.Context) (string, error) {
	if f.unsavedChoice == "" {
		return "Cancel", nil
	}
	if f.unsavedChoice == "error" {
		return "", errors.New("dialog failed")
	}
	return f.unsavedChoice, nil
}

func (f *fakeNative) SetTitle(_ context.Context, title string) {
	f.title = title
}

type fakeDocuments struct {
	read      map[string]string
	writes    map[string]string
	readErr   error
	writeErr  error
	readCount int
}

func (f *fakeDocuments) ReadMarkdown(path string) (string, error) {
	f.readCount++
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.read[path], nil
}

func (f *fakeDocuments) WriteMarkdown(path, content string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.writes == nil {
		f.writes = map[string]string{}
	}
	f.writes[path] = content
	return nil
}

type fakeFonts struct {
	families []string
}

func (f fakeFonts) ListFamilies() []string {
	if f.families == nil {
		return []string{"Menlo"}
	}
	return f.families
}

type fakePreferences struct {
	loaded   preferences.Preferences
	saved    preferences.Preferences
	recorded []string
}

func (f *fakePreferences) Load() (preferences.Preferences, error) {
	return f.loaded, nil
}

func (f *fakePreferences) Save(prefs preferences.Preferences) error {
	f.saved = prefs
	return nil
}

func (f *fakePreferences) RecordRecent(path string) ([]preferences.RecentDocument, error) {
	f.recorded = append(f.recorded, path)
	return []preferences.RecentDocument{{Path: path, Title: path}}, nil
}
