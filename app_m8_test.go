package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	imageassets "dr-markdown/internal/assets"
)

func TestAppImportImageUsesNativePickerAndAssetImporter(t *testing.T) {
	native := &fakeNative{imagePath: "/tmp/source.png"}
	importer := &fakeImageImporter{
		result: imageassets.ImportedImage{Markdown: "![source](doc.assets/source.png)"},
	}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      importer,
	})
	app.startup(context.Background())

	result, err := app.ImportImage("/tmp/doc.md")
	if err != nil {
		t.Fatalf("ImportImage returned error: %v", err)
	}
	if importer.documentPath != "/tmp/doc.md" || importer.sourcePath != "/tmp/source.png" {
		t.Fatalf("importer paths = %q %q", importer.documentPath, importer.sourcePath)
	}
	if result.Markdown != "![source](doc.assets/source.png)" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAppImportImageCancelDoesNotImport(t *testing.T) {
	importer := &fakeImageImporter{}
	app := newAppWithDependencies(appDependencies{
		native:      &fakeNative{},
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      importer,
	})

	result, err := app.ImportImage("/tmp/doc.md")
	if err != nil {
		t.Fatalf("ImportImage cancel returned error: %v", err)
	}
	if result != (imageassets.ImportedImage{}) {
		t.Fatalf("cancel result = %#v", result)
	}
	if importer.called {
		t.Fatal("cancel should not call the asset importer")
	}
}

func TestAppImportImageReportsImporterError(t *testing.T) {
	native := &fakeNative{imagePath: "/tmp/source.png"}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      &fakeImageImporter{err: errors.New("copy failed")},
	})

	if _, err := app.ImportImage("/tmp/doc.md"); err == nil {
		t.Fatal("ImportImage should return importer errors")
	}
	if native.errorTitle != "Image Import Failed" || native.errorMessage != "copy failed" {
		t.Fatalf("native error = %q %q", native.errorTitle, native.errorMessage)
	}
}

// An unsaved document can never yield a portable relative asset path, so the
// rejection must happen before the user is asked to choose a file.
func TestAppImportImageRejectsUnsavedDocumentBeforePrompting(t *testing.T) {
	native := &fakeNative{imagePath: "/tmp/source.png"}
	importer := &fakeImageImporter{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      importer,
	})

	if _, err := app.ImportImage(""); err == nil {
		t.Fatal("ImportImage should reject an unsaved document")
	}
	if native.imageCalled {
		t.Error("unsaved document should not open the image picker")
	}
	if importer.called {
		t.Error("unsaved document should not reach the asset importer")
	}
	if native.errorTitle != "Image Import Failed" {
		t.Errorf("native error title = %q", native.errorTitle)
	}
	if !strings.Contains(native.errorMessage, "Save the document") {
		t.Errorf("native error message = %q, want it to tell the user to save first", native.errorMessage)
	}
}

func TestAppLoadImageAssetRoutesThroughAssetPort(t *testing.T) {
	importer := &fakeImageImporter{
		loaded: imageassets.LoadedImage{DataURI: "data:image/png;base64,AAAA", Exists: true},
	}
	app := newAppWithDependencies(appDependencies{
		native:      &fakeNative{},
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      importer,
	})

	loaded, err := app.LoadImageAsset("/tmp/doc.md", "doc.assets/photo.png")
	if err != nil {
		t.Fatalf("LoadImageAsset returned error: %v", err)
	}
	if importer.loadDocumentPath != "/tmp/doc.md" || importer.loadMarkdownPath != "doc.assets/photo.png" {
		t.Fatalf("load paths = %q %q", importer.loadDocumentPath, importer.loadMarkdownPath)
	}
	if loaded.DataURI != "data:image/png;base64,AAAA" {
		t.Fatalf("loaded = %#v", loaded)
	}
}

// Revealing a missing asset must tell the user rather than silently doing
// nothing or asking the OS to reveal a path that is not there.
func TestAppRevealImageAssetReportsMissingAsset(t *testing.T) {
	native := &fakeNative{}
	importer := &fakeImageImporter{loaded: imageassets.LoadedImage{AbsolutePath: "/tmp/gone.png"}}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      importer,
	})

	if err := app.RevealImageAsset("/tmp/doc.md", "doc.assets/gone.png"); err == nil {
		t.Fatal("revealing a missing asset should return an error")
	}
	if native.revealedPath != "" {
		t.Errorf("missing asset should not be revealed, got %q", native.revealedPath)
	}
	if native.errorTitle == "" {
		t.Error("missing asset should surface a native error")
	}
}

func TestAppRevealImageAssetRevealsExistingAsset(t *testing.T) {
	native := &fakeNative{}
	importer := &fakeImageImporter{
		loaded: imageassets.LoadedImage{AbsolutePath: "/tmp/doc.assets/photo.png", Exists: true},
	}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      importer,
	})

	if err := app.RevealImageAsset("/tmp/doc.md", "doc.assets/photo.png"); err != nil {
		t.Fatalf("RevealImageAsset returned error: %v", err)
	}
	if native.revealedPath != "/tmp/doc.assets/photo.png" {
		t.Fatalf("revealed path = %q", native.revealedPath)
	}
}

// Dropped files arrive with a path already chosen, so the picker must be
// skipped, but every other import rule still applies.
func TestAppImportDroppedImageSkipsPickerAndAppliesAssetPolicy(t *testing.T) {
	native := &fakeNative{}
	importer := &fakeImageImporter{
		result: imageassets.ImportedImage{Markdown: "![dropped](doc.assets/dropped.png)"},
	}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      importer,
	})
	app.startup(context.Background())

	result, err := app.ImportDroppedImage("/tmp/doc.md", "/tmp/dropped.png")
	if err != nil {
		t.Fatalf("ImportDroppedImage returned error: %v", err)
	}
	if native.imageCalled {
		t.Error("a dropped file already has a path; the picker should not open")
	}
	if importer.sourcePath != "/tmp/dropped.png" || importer.documentPath != "/tmp/doc.md" {
		t.Fatalf("importer paths = %q %q", importer.documentPath, importer.sourcePath)
	}
	if result.Markdown != "![dropped](doc.assets/dropped.png)" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAppImportDroppedImageRejectsUnsavedDocument(t *testing.T) {
	native := &fakeNative{}
	importer := &fakeImageImporter{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      importer,
	})

	if _, err := app.ImportDroppedImage("", "/tmp/dropped.png"); err == nil {
		t.Fatal("dropping onto an unsaved document should be rejected")
	}
	if importer.called {
		t.Error("unsaved document should not reach the asset importer")
	}
	if native.errorTitle == "" {
		t.Error("rejection should surface a native error")
	}
}

func TestAppImportDroppedImageReportsImporterError(t *testing.T) {
	native := &fakeNative{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      &fakeImageImporter{err: errors.New("copy failed")},
	})

	if _, err := app.ImportDroppedImage("/tmp/doc.md", "/tmp/dropped.png"); err == nil {
		t.Fatal("ImportDroppedImage should return importer errors")
	}
	if native.errorTitle != "Image Import Failed" || native.errorMessage != "copy failed" {
		t.Fatalf("native error = %q %q", native.errorTitle, native.errorMessage)
	}
}

func TestAppRevealImageAssetReportsLoadError(t *testing.T) {
	native := &fakeNative{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
		images:      &fakeImageImporter{loadErr: errors.New("not a local asset")},
	})

	if err := app.RevealImageAsset("/tmp/doc.md", "https://example.com/photo.png"); err == nil {
		t.Fatal("RevealImageAsset should return loader errors")
	}
	if native.errorTitle != "Reveal Failed" || native.errorMessage != "not a local asset" {
		t.Fatalf("native error = %q %q", native.errorTitle, native.errorMessage)
	}
	if native.revealedPath != "" {
		t.Errorf("failed load should not reveal anything, got %q", native.revealedPath)
	}
}

type fakeImageImporter struct {
	documentPath     string
	sourcePath       string
	loadDocumentPath string
	loadMarkdownPath string
	result           imageassets.ImportedImage
	loaded           imageassets.LoadedImage
	err              error
	loadErr          error
	called           bool
}

func (f *fakeImageImporter) ImportForDocument(documentPath string, sourcePath string) (imageassets.ImportedImage, error) {
	f.called = true
	f.documentPath = documentPath
	f.sourcePath = sourcePath
	return f.result, f.err
}

func (f *fakeImageImporter) LoadForDocument(documentPath string, markdownPath string) (imageassets.LoadedImage, error) {
	f.loadDocumentPath = documentPath
	f.loadMarkdownPath = markdownPath
	return f.loaded, f.loadErr
}
