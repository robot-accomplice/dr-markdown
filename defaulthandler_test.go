package main

import (
	"errors"
	"testing"
)

// The menu item's whole decision, as a pure function of two facts the OS owns:
// whether we are already the default, and where the running executable lives.
func TestDefaultHandlerMenuState(t *testing.T) {
	cases := []struct {
		name      string
		isDefault bool
		execPath  string
		want      int
	}{
		{"offer from a normal install", false, "/Applications/Dr Markdown.app/Contents/MacOS/Dr Markdown", defaultHandlerOffer},
		{"checked and disabled when already default", true, "/Applications/Dr Markdown.app/Contents/MacOS/Dr Markdown", defaultHandlerIsDefault},
		{"disabled when running from a mounted image", false, "/Volumes/Dr Markdown/Dr Markdown.app/Contents/MacOS/Dr Markdown", defaultHandlerDiskImage},
		{"disabled when running translocated by Gatekeeper", false, "/private/var/folders/xx/yyy/AppTranslocation/ABC/d/Dr Markdown.app/Contents/MacOS/Dr Markdown", defaultHandlerDiskImage},
		{"already-default wins over disk image", true, "/Volumes/Dr Markdown/Dr Markdown.app/Contents/MacOS/Dr Markdown", defaultHandlerIsDefault},
		{"a home-directory install is fine", false, "/Users/u/Applications/Dr Markdown.app/Contents/MacOS/Dr Markdown", defaultHandlerOffer},
		{"a bare gate binary outside any bundle offers", false, "/tmp/drmd-gate", defaultHandlerOffer},
		{"an unreadable executable path still offers", false, "", defaultHandlerOffer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultHandlerMenuState(tc.isDefault, tc.execPath); got != tc.want {
				t.Errorf("defaultHandlerMenuState(%v, %q) = %d, want %d", tc.isDefault, tc.execPath, got, tc.want)
			}
		})
	}
}

func defaultHandlerApp(native *fakeNative) *App {
	return newAppWithDependencies(appDependencies{
		native:      native,
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
}

func TestSetAsDefaultMarkdownHandlerAsksTheOS(t *testing.T) {
	native := &fakeNative{}
	defaultHandlerApp(native).SetAsDefaultMarkdownHandler()
	if !native.setDefaultCalled {
		t.Error("SetDefaultMarkdownHandler was not called on the native port")
	}
}

func TestSetAsDefaultMarkdownHandlerDoesNothingWhenAlreadyDefault(t *testing.T) {
	native := &fakeNative{isDefault: true}
	defaultHandlerApp(native).SetAsDefaultMarkdownHandler()
	if native.setDefaultCalled {
		t.Error("set was called while already the default; the offer should be a no-op")
	}
}

func TestSetAsDefaultMarkdownHandlerRefusesADiskImage(t *testing.T) {
	native := &fakeNative{}
	defer func(orig func() (string, error)) { osExecutable = orig }(osExecutable)
	osExecutable = func() (string, error) {
		return "/Volumes/Dr Markdown/Dr Markdown.app/Contents/MacOS/Dr Markdown", nil
	}
	defaultHandlerApp(native).SetAsDefaultMarkdownHandler()
	if native.setDefaultCalled {
		t.Error("set was called from a mounted disk image; that association breaks on eject")
	}
	if native.errorCount == 0 {
		t.Error("the refusal was silent; the user must be told to drag the app to Applications")
	}
}

func TestSetAsDefaultMarkdownHandlerReportsANativeFailure(t *testing.T) {
	native := &fakeNative{setDefaultErr: errors.New("boom")}
	defaultHandlerApp(native).SetAsDefaultMarkdownHandler()
	if native.errorCount == 0 {
		t.Error("a Launch Services failure reached nobody")
	}
}
