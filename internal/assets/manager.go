// Package assets manages imported document assets.
package assets

import (
	"fmt"
	"io"
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
		Markdown:     fmt.Sprintf("![%s](%s)", alt, markdownPath),
	}, nil
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
