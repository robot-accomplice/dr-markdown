package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestIconFreeMessageDialogPNGIsValidTransparentImage(t *testing.T) {
	if len(iconFreeMessageDialogPNG) == 0 {
		t.Fatal("message dialog icon override must not be empty")
	}
	img, err := png.Decode(bytes.NewReader(iconFreeMessageDialogPNG))
	if err != nil {
		t.Fatalf("message dialog icon override should be a valid PNG: %v", err)
	}
	if img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 {
		t.Fatalf("message dialog icon override should be 1x1, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
	_, _, _, alpha := img.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatalf("message dialog icon override should be transparent, alpha=%d", alpha)
	}
}
