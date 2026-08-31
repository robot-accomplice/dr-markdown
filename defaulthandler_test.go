package main

import "testing"

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
