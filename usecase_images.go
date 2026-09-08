package main

import (
	"context"
	"errors"
	"fmt"

	imageassets "dr-markdown/internal/assets"
)

// imageUseCase owns asset import, load and reveal. The unsaved-document
// rejection lives here so the picker never opens for an import that could
// not produce a portable relative path.
type imageUseCase struct {
	native nativePort
	images imageAssetPort
}

// errUnsavedImageImport rejects imports that cannot produce a portable
// relative asset path because the document has no location on disk yet.
var errUnsavedImageImport = errors.New("Save the document before inserting images.")

// importViaPicker selects an image, copies it into the document asset folder,
// and returns markdown for insertion. A canceled picker returns an empty
// result. An unsaved document is rejected before the picker opens, so the
// user is never asked to choose a file the import could never have accepted.
func (uc *imageUseCase) importViaPicker(ctx context.Context, documentPath string) (imageassets.ImportedImage, error) {
	if documentPath == "" {
		uc.native.ShowError(ctx, "Image Import Failed", errUnsavedImageImport.Error())
		return imageassets.ImportedImage{}, errUnsavedImageImport
	}
	sourcePath, err := uc.native.SelectImageFile(ctx)
	if err != nil {
		return imageassets.ImportedImage{}, err
	}
	if sourcePath == "" {
		return imageassets.ImportedImage{}, nil
	}
	result, err := uc.images.ImportForDocument(documentPath, sourcePath)
	if err != nil {
		uc.native.ShowError(ctx, "Image Import Failed", err.Error())
		return imageassets.ImportedImage{}, err
	}
	return result, nil
}

// importDropped imports a file the user dropped onto the window. The
// path is already known, so no picker is shown, but the same asset policy and
// unsaved-document rejection apply as for the ribbon command.
func (uc *imageUseCase) importDropped(ctx context.Context, documentPath string, sourcePath string) (imageassets.ImportedImage, error) {
	if documentPath == "" {
		uc.native.ShowError(ctx, "Image Import Failed", errUnsavedImageImport.Error())
		return imageassets.ImportedImage{}, errUnsavedImageImport
	}
	result, err := uc.images.ImportForDocument(documentPath, sourcePath)
	if err != nil {
		uc.native.ShowError(ctx, "Image Import Failed", err.Error())
		return imageassets.ImportedImage{}, err
	}
	return result, nil
}

// load inlines a document-relative image so the webview can render
// it and so print/export artifacts stay self-contained.
func (uc *imageUseCase) load(ctx context.Context, documentPath string, markdownPath string) (imageassets.LoadedImage, error) {
	return uc.images.LoadForDocument(documentPath, markdownPath)
}

// reveal shows an image asset in the OS file browser. A missing
// asset is reported instead of silently doing nothing.
func (uc *imageUseCase) reveal(ctx context.Context, documentPath string, markdownPath string) error {
	loaded, err := uc.images.LoadForDocument(documentPath, markdownPath)
	if err != nil {
		uc.native.ShowError(ctx, "Reveal Failed", err.Error())
		return err
	}
	if !loaded.Exists {
		err := fmt.Errorf("image asset is missing: %s", markdownPath)
		uc.native.ShowError(ctx, "Reveal Failed", err.Error())
		return err
	}
	if err := uc.native.RevealPath(ctx, loaded.AbsolutePath); err != nil {
		uc.native.ShowError(ctx, "Reveal Failed", err.Error())
		return err
	}
	return nil
}
