package assets

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
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

func TestImportForDocumentRejectsUnreadableAndDirectorySources(t *testing.T) {
	root := t.TempDir()
	docPath := filepath.Join(root, "notes.md")

	if _, err := ImportForDocument(docPath, filepath.Join(root, "absent.png")); err == nil {
		t.Error("a source that does not exist should be rejected")
	}

	dir := filepath.Join(root, "folder.png")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create dir source: %v", err)
	}
	if _, err := ImportForDocument(docPath, dir); err == nil {
		t.Error("a directory source should be rejected")
	}
}

// Relative markdown image paths resolve against the document directory, not the
// webview asset-server origin, so rendering and export both need the bytes
// inlined as a data URI.
func TestLoadForDocumentInlinesRelativeAssetAsDataURI(t *testing.T) {
	root := t.TempDir()
	assetDir := filepath.Join(root, "notes.assets")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		t.Fatalf("create asset dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "photo.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	loaded, err := LoadForDocument(filepath.Join(root, "notes.md"), "notes.assets/photo.png")
	if err != nil {
		t.Fatalf("LoadForDocument returned error: %v", err)
	}
	if !loaded.Exists {
		t.Fatal("asset should be reported as existing")
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	if loaded.DataURI != want {
		t.Fatalf("DataURI = %q, want %q", loaded.DataURI, want)
	}
}

// A missing asset is a render state the editor must show, not a hard error.
func TestLoadForDocumentReportsMissingAssetWithoutError(t *testing.T) {
	root := t.TempDir()
	loaded, err := LoadForDocument(filepath.Join(root, "notes.md"), "notes.assets/gone.png")
	if err != nil {
		t.Fatalf("missing asset should not error: %v", err)
	}
	if loaded.Exists {
		t.Fatal("missing asset should not report Exists")
	}
	if loaded.DataURI != "" {
		t.Fatalf("missing asset should carry no data, got %q", loaded.DataURI)
	}
}

// Remote and data-URI sources are already renderable by the webview and must
// not be treated as local assets.
func TestLoadForDocumentRejectsNonLocalSources(t *testing.T) {
	docPath := filepath.Join(t.TempDir(), "notes.md")
	for _, markdownPath := range []string{"https://example.com/photo.png", "data:image/png;base64,AAAA"} {
		if _, err := LoadForDocument(docPath, markdownPath); err == nil {
			t.Fatalf("%q should be rejected as a non-local asset", markdownPath)
		}
	}
	if _, err := LoadForDocument("", "notes.assets/photo.png"); err == nil {
		t.Fatal("unsaved document cannot resolve a relative asset")
	}
}

func TestLoadForDocumentMapsExtensionToMimeType(t *testing.T) {
	root := t.TempDir()
	for name, wantMime := range map[string]string{
		"a.png":  "image/png",
		"b.jpg":  "image/jpeg",
		"c.jpeg": "image/jpeg",
		"d.gif":  "image/gif",
		"e.webp": "image/webp",
		"f.svg":  "image/svg+xml",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		loaded, err := LoadForDocument(filepath.Join(root, "notes.md"), name)
		if err != nil {
			t.Fatalf("LoadForDocument(%s): %v", name, err)
		}
		if !strings.HasPrefix(loaded.DataURI, "data:"+wantMime+";base64,") {
			t.Errorf("%s DataURI = %q, want mime %q", name, loaded.DataURI, wantMime)
		}
	}
}
