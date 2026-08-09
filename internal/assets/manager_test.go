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

// An opened document is untrusted input — the product exists to view arbitrary
// markdown, including files the user did not write. A document must therefore
// not be able to name a path outside its own directory and have the editor
// read it; the render pass resolves every image automatically, with no user
// interaction, so this is reachable purely by opening a file.
func TestLoadForDocumentRefusesPathsOutsideTheDocumentDirectory(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "notes")
	if err := os.MkdirAll(docDir, 0o700); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(docDir, "note.md")

	secretDir := filepath.Join(root, "private")
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(secretDir, "id_rsa")
	if err := os.WriteFile(secret, []byte("SENSITIVE"), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, markdownPath := range map[string]string{
		"absolute path":        secret,
		"parent traversal":     "../private/id_rsa",
		"nested traversal":     "assets/../../private/id_rsa",
		"absolute system path": "/etc/hosts",
	} {
		loaded, err := LoadForDocument(docPath, markdownPath)
		if err == nil {
			t.Errorf("%s: expected refusal, got none", name)
		}
		if loaded.DataURI != "" {
			t.Errorf("%s: refused path still returned %d bytes of data", name, len(loaded.DataURI))
		}
	}
}

// Containment must not break the assets the importer itself produces.
func TestLoadForDocumentStillLoadsItsOwnAssetFolder(t *testing.T) {
	root := t.TempDir()
	docPath := filepath.Join(root, "notes.md")
	assetDir := filepath.Join(root, "notes.assets")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "photo.png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadForDocument(docPath, "notes.assets/photo.png")
	if err != nil || !loaded.Exists {
		t.Fatalf("the importer's own asset layout must still load: err=%v exists=%v", err, loaded.Exists)
	}
}

// A symlink inside the document directory must not be a way around containment.
func TestLoadForDocumentRefusesSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "notes")
	os.MkdirAll(docDir, 0o700)
	docPath := filepath.Join(docDir, "note.md")
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("SENSITIVE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(docDir, "innocent.png")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	loaded, err := LoadForDocument(docPath, "innocent.png")
	if err == nil && loaded.DataURI != "" {
		t.Fatal("a symlink pointing outside the document directory must not be followed")
	}
}

// The default macOS screenshot name contains spaces, which are illegal in a
// CommonMark destination. The importer emitted them raw, so the app's own
// headline feature wrote a broken link into the user's document and saved it.
func TestImportForDocumentEmitsAValidMarkdownReference(t *testing.T) {
	root := t.TempDir()
	docPath := filepath.Join(root, "notes.md")

	for _, name := range []string{
		"Screen Shot 2026-08-07 at 10.02.11.png",
		"a(b)c.png",
		"square[bracket].png",
	} {
		source := filepath.Join(root, name)
		if err := os.WriteFile(source, []byte("png"), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := ImportForDocument(docPath, source)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		dest := result.Markdown[strings.Index(result.Markdown, "](")+2 : len(result.Markdown)-1]
		if strings.ContainsAny(dest, " ()") {
			t.Errorf("%s: destination is not a valid CommonMark link: %q", name, result.Markdown)
		}
		alt := result.Markdown[2:strings.Index(result.Markdown, "](")]
		if strings.Contains(alt, "[") && !strings.Contains(alt, `\[`) {
			t.Errorf("%s: unescaped bracket in alt text: %q", name, result.Markdown)
		}
	}
}

// Import and render are two halves of one feature. The importer now
// percent-encodes the destination, so the loader must decode it or every
// imported image with a space in its name renders as missing — the reference
// would be valid markdown pointing at a file that does not exist.
func TestImportedImageRoundTripsThroughTheLoader(t *testing.T) {
	root := t.TempDir()
	docPath := filepath.Join(root, "notes.md")
	source := filepath.Join(root, "Screen Shot 2026-08-07 at 10.02.11.png")
	if err := os.WriteFile(source, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportForDocument(docPath, source)
	if err != nil {
		t.Fatal(err)
	}
	dest := imported.Markdown[strings.Index(imported.Markdown, "](")+2 : len(imported.Markdown)-1]

	loaded, err := LoadForDocument(docPath, dest)
	if err != nil {
		t.Fatalf("load %q: %v", dest, err)
	}
	if !loaded.Exists {
		t.Errorf("the image just imported does not render: %q -> %q", dest, loaded.AbsolutePath)
	}
}

// Decoding must happen BEFORE containment, or percent-encoding becomes the
// bypass: `%2e%2e%2f` is `../` and a containment check on the encoded string
// sees an ordinary filename. This app opens arbitrary markdown, so the document
// choosing the path is untrusted input.
func TestPercentEncodingCannotSmuggleTraversalPastContainment(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "vault")
	if err := os.MkdirAll(docDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, "secret.png")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadForDocument(filepath.Join(docDir, "notes.md"), "%2e%2e%2fsecret.png")
	if err == nil {
		t.Fatalf("encoded traversal was accepted: %#v", loaded)
	}
	if strings.Contains(loaded.DataURI, base64.StdEncoding.EncodeToString([]byte("private"))) {
		t.Error("the file outside the document directory was read")
	}
}

// Containment bounded the DIRECTORY but not the file TYPE: any extension fell
// back to application/octet-stream and its bytes were inlined anyway. Rendering
// is automatic and needs no user interaction, so opening a markdown file from a
// Downloads folder or a repository root turned "view this document" into "read
// the interesting files next to it". This product opens ARBITRARY markdown, so
// the document choosing the path is untrusted input.
func TestOnlyImageFilesAreInlined(t *testing.T) {
	root := t.TempDir()
	docPath := filepath.Join(root, "notes.md")
	secret := "AWS_SECRET_ACCESS_KEY=hunter2"

	for _, name := range []string{".env", "secrets.yaml", "id_rsa", "config.json", "notes.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(secret), 0o600); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadForDocument(docPath, name)
		if err == nil {
			t.Errorf("%s: a non-image file was accepted", name)
		}
		if strings.Contains(loaded.DataURI, base64.StdEncoding.EncodeToString([]byte(secret))) {
			t.Errorf("%s: file contents were inlined into the document", name)
		}
	}

	// A real image beside the document must still load.
	if err := os.WriteFile(filepath.Join(root, "photo.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadForDocument(docPath, "photo.png")
	if err != nil || !loaded.Exists {
		t.Errorf("an ordinary image must still load: err=%v exists=%v", err, loaded.Exists)
	}
}
