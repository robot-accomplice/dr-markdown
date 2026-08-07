package main

import (
	"context"
	"errors"
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
		images:      &fakeImageImporter{err: errors.New("save first")},
	})

	if _, err := app.ImportImage(""); err == nil {
		t.Fatal("ImportImage should return importer errors")
	}
	if native.errorTitle != "Image Import Failed" || native.errorMessage != "save first" {
		t.Fatalf("native error = %q %q", native.errorTitle, native.errorMessage)
	}
}

type fakeImageImporter struct {
	documentPath string
	sourcePath   string
	result       imageassets.ImportedImage
	err          error
	called       bool
}

func (f *fakeImageImporter) ImportForDocument(documentPath string, sourcePath string) (imageassets.ImportedImage, error) {
	f.called = true
	f.documentPath = documentPath
	f.sourcePath = sourcePath
	return f.result, f.err
}
