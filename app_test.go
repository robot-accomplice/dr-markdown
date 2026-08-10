package main

import (
	"bytes"
	"context"
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

// The two directions are separate mechanisms under any host: one receives from
// the OS, the other sends to the webview. Fusing them in a single
// SubscribeFileDrop hid a host dependency inside what read as a subscription.
func TestFileDropSubscriptionAndEmissionAreSeparate(t *testing.T) {
	native := &fakeNative{}
	app := newAppWithDependencies(appDependencies{native: native})
	app.ctx = context.Background()

	app.startup(app.ctx)

	if native.dropHandler == nil {
		t.Fatal("startup did not subscribe to file drops")
	}
	native.dropHandler([]string{"/tmp/a.md", "/tmp/b.md"})

	if got := native.emittedDrops; len(got) != 1 || got[0][0] != "/tmp/a.md" {
		t.Errorf("dropped paths were not emitted to the frontend: %v", got)
	}
}
