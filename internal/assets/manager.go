// Package assets manages imported document assets.
package assets

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ImportedImage describes a copied image asset and its markdown reference.
type ImportedImage struct {
	SourcePath   string `json:"sourcePath"`
	AssetPath    string `json:"assetPath"`
	MarkdownPath string `json:"markdownPath"`
	Markdown     string `json:"markdown"`
}

// LoadedImage describes a local image asset resolved against a document.
type LoadedImage struct {
	AbsolutePath string `json:"absolutePath"`
	DataURI      string `json:"dataURI"`
	Exists       bool   `json:"exists"`
}

// imageMimeTypes maps the asset extensions the importer accepts to the MIME
// type used when inlining them.
var imageMimeTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// LoadForDocument resolves a markdown image path against the document
// directory and inlines the bytes as a data URI.
//
// Markdown resolves relative image paths against the document on disk, but the
// webview resolves them against the asset-server origin, so a copied asset
// never renders from its relative path alone. Inlining serves both the editor
// surface and print/export, which must be self-contained.
//
// A missing file is a render state the editor shows, not an error; an
// unreadable file is a real failure and is reported.
func LoadForDocument(documentPath string, markdownPath string) (LoadedImage, error) {
	if documentPath == "" {
		return LoadedImage{}, fmt.Errorf("load image: document has no location on disk")
	}
	if markdownPath == "" {
		return LoadedImage{}, fmt.Errorf("load image: empty asset path")
	}
	// A markdown destination is URL-encoded, so `%20` denotes a space on disk.
	// Decode BEFORE every check that reads the path, never after: `%2e%2e%2f`
	// decodes to `../`, and a containment check run on the encoded form would
	// pass it through. A filename holding a literal `%` fails to unescape and is
	// used verbatim.
	if decoded, err := url.PathUnescape(markdownPath); err == nil {
		markdownPath = decoded
	}
	if isNonLocalSource(markdownPath) {
		return LoadedImage{}, fmt.Errorf("load image: %q is not a local asset", markdownPath)
	}

	// Containment. This application exists to view ARBITRARY markdown, so an
	// opened document is untrusted input, and resolveImageAssets loads every
	// image reference automatically on render with no user interaction. An
	// unconstrained path therefore turns "open this file" into "read any file
	// this user can read" — absolute paths, `..` traversal and non-image
	// extensions all worked, and the bytes landed in the DOM as a data URI.
	//
	// The trade-off is deliberate: a document may only reference assets at or
	// below its own directory. That refuses legitimate layouts like
	// `../shared/logo.png`, which is the cost of not reading a stranger's
	// choice of path. Refusals surface through the existing missing-asset
	// render state rather than failing the document.
	if filepath.IsAbs(markdownPath) {
		return LoadedImage{}, fmt.Errorf("load image: absolute paths are not read from documents: %s", markdownPath)
	}
	docDir := filepath.Dir(documentPath)
	absolute := filepath.Join(docDir, filepath.FromSlash(markdownPath))
	if !withinDirectory(docDir, absolute) {
		return LoadedImage{}, fmt.Errorf("load image: %q resolves outside the document directory", markdownPath)
	}
	// Re-check after following links, so a symlink inside the directory cannot
	// be used as a door out of it. BOTH sides must be resolved before
	// comparing: on macOS the document directory itself frequently sits under
	// a symlink (/var -> /private/var), and comparing a resolved target against
	// an unresolved base rejects perfectly ordinary paths.
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		base := docDir
		if resolvedDir, err := filepath.EvalSymlinks(docDir); err == nil {
			base = resolvedDir
		}
		if !withinDirectory(base, resolved) {
			return LoadedImage{}, fmt.Errorf("load image: %q links outside the document directory", markdownPath)
		}
	}
	info, err := os.Stat(absolute)
	if os.IsNotExist(err) || (err == nil && info.IsDir()) {
		return LoadedImage{AbsolutePath: absolute}, nil
	}
	if err != nil {
		return LoadedImage{AbsolutePath: absolute}, fmt.Errorf("stat image asset: %w", err)
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return LoadedImage{AbsolutePath: absolute}, fmt.Errorf("read image asset: %w", err)
	}

	mime, ok := imageMimeTypes[strings.ToLower(filepath.Ext(absolute))]
	if !ok {
		mime = "application/octet-stream"
	}
	return LoadedImage{
		AbsolutePath: absolute,
		DataURI:      "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
		Exists:       true,
	}, nil
}

// isNonLocalSource reports paths the webview can already render itself.
func isNonLocalSource(markdownPath string) bool {
	lower := strings.ToLower(markdownPath)
	for _, prefix := range []string{"http://", "https://", "data:", "file://"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// ImportForDocument copies sourcePath beside documentPath and returns markdown.
func ImportForDocument(documentPath string, sourcePath string) (ImportedImage, error) {
	if documentPath == "" {
		return ImportedImage{}, fmt.Errorf("import image: save the document before importing images")
	}
	if sourcePath == "" {
		return ImportedImage{}, fmt.Errorf("import image: empty source path")
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return ImportedImage{}, fmt.Errorf("stat image source: %w", err)
	}
	if info.IsDir() {
		return ImportedImage{}, fmt.Errorf("import image: source is a directory")
	}

	docDir := filepath.Dir(documentPath)
	docName := strings.TrimSuffix(filepath.Base(documentPath), filepath.Ext(documentPath))
	assetDir := filepath.Join(docDir, docName+".assets")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		return ImportedImage{}, fmt.Errorf("create asset directory: %w", err)
	}

	target := uniqueTargetPath(assetDir, filepath.Base(sourcePath))
	if err := copyFile(target, sourcePath); err != nil {
		return ImportedImage{}, err
	}
	rel, err := filepath.Rel(docDir, target)
	if err != nil {
		return ImportedImage{}, fmt.Errorf("make relative asset path: %w", err)
	}
	markdownPath := filepath.ToSlash(rel)
	alt := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	return ImportedImage{
		SourcePath:   sourcePath,
		AssetPath:    target,
		MarkdownPath: markdownPath,
		Markdown:     fmt.Sprintf("![%s](%s)", escapeLinkText(alt), escapeLinkDestination(markdownPath)),
	}, nil
}

// escapeLinkDestination percent-encodes everything outside the unreserved set,
// keeping `/` as the path separator.
//
// The importer built the reference with raw Sprintf, and the default macOS
// screenshot filename contains spaces — so importing one produced
// `![...](notes.assets/Screen Shot 2026-08-07 at 10.02.11.png)`, which no
// CommonMark parser reads as a link. The app's own image feature wrote broken
// markdown into the document and then saved it. Parentheses close the
// destination early and break it the same way, so the conservative allowlist is
// the fix rather than a space-only substitution.
func escapeLinkDestination(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '.', c == '_', c == '~', c == '/':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// escapeLinkText escapes the characters that end an alt-text span early.
func escapeLinkText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`)
	return replacer.Replace(text)
}

func uniqueTargetPath(dir string, filename string) string {
	ext := filepath.Ext(filename)
	stem := strings.TrimSuffix(filename, ext)
	target := filepath.Join(dir, filename)
	for i := 1; ; i++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			return target
		}
		target = filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext))
	}
}

func copyFile(target string, source string) error {
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open image source: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create image asset: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(target)
		return fmt.Errorf("copy image asset: %w", err)
	}
	return nil
}

// withinDirectory reports whether target is dir itself or below it.
func withinDirectory(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
