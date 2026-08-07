package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportForSavedDocumentCopiesImageAndReturnsPortableMarkdown(t *testing.T) {
	root := t.TempDir()
	docPath := filepath.Join(root, "notes.md")
	source := filepath.Join(root, "photo.png")
	if err := os.WriteFile(source, []byte("png"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	result, err := ImportForDocument(docPath, source)
	if err != nil {
		t.Fatalf("ImportForDocument returned error: %v", err)
	}
	if result.MarkdownPath != "notes.assets/photo.png" {
		t.Fatalf("MarkdownPath = %q", result.MarkdownPath)
	}
	if result.Markdown != "![photo](notes.assets/photo.png)" {
		t.Fatalf("Markdown = %q", result.Markdown)
	}
	if got, err := os.ReadFile(filepath.Join(root, result.MarkdownPath)); err != nil || string(got) != "png" {
		t.Fatalf("copied asset mismatch: %q %v", got, err)
	}
}

func TestImportForDocumentAddsCollisionSuffix(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "photo.png")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	assetDir := filepath.Join(root, "notes.assets")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		t.Fatalf("create asset dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "photo.png"), []byte("old"), 0o600); err != nil {
		t.Fatalf("write collision: %v", err)
	}

	result, err := ImportForDocument(filepath.Join(root, "notes.md"), source)
	if err != nil {
		t.Fatalf("ImportForDocument returned error: %v", err)
	}
	if result.MarkdownPath != "notes.assets/photo-1.png" {
		t.Fatalf("MarkdownPath = %q", result.MarkdownPath)
	}
}

func TestImportForDocumentRejectsUnsavedDocumentAndMissingSource(t *testing.T) {
	if _, err := ImportForDocument("", "/tmp/photo.png"); err == nil {
		t.Fatal("unsaved documents should not import images")
	}
	if _, err := ImportForDocument(filepath.Join(t.TempDir(), "notes.md"), ""); err == nil {
		t.Fatal("empty source should be rejected")
	}
}
