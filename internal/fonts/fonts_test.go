package fonts

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestFamilyNameNormalizesFontFilenames(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/System/Library/Fonts/Menlo.ttc", "Menlo"},
		{"/System/Library/Fonts/Supplemental/Times New Roman Bold Italic.ttf", "Times New Roman"},
		{"/Library/Fonts/Fira_Code_Regular.otf", "Fira Code"},
		{"/Library/Fonts/readme.txt", ""},
	} {
		if got := familyName(tc.path); got != tc.want {
			t.Fatalf("familyName(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestListFamiliesReturnsSortedUniqueFontFamilies(t *testing.T) {
	dir := t.TempDir()
	originalDirs := fontDirs
	fontDirs = []string{dir}
	t.Cleanup(func() { fontDirs = originalDirs })

	for _, name := range []string{
		"Fira Code Regular.ttf",
		"Fira Code Bold.ttf",
		"Menlo.ttc",
		"notes.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("font"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := ListFamilies("")
	want := []string{"Fira Code", "Menlo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListFamilies() = %#v, want %#v", got, want)
	}
}

func TestListFamiliesIncludesUserFontDirectory(t *testing.T) {
	home := t.TempDir()
	// Build the directory the CURRENT platform actually looks in, rather than
	// hardcoding macOS's. The literal "Library/Fonts" made this test pass only
	// on the machine it was written on: production code has been correctly
	// platform-aware since the cross-platform pass, but the test pinned the
	// author's OS and went red the first time CI ran it on Linux.
	userFonts := userFontDir(runtime.GOOS, home)
	if userFonts == "" {
		t.Skipf("%s has no per-user font directory; system dirs cover it", runtime.GOOS)
	}
	if err := os.MkdirAll(userFonts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userFonts, "User Font.otf"), []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalDirs := fontDirs
	fontDirs = []string{}
	t.Cleanup(func() { fontDirs = originalDirs })

	got := ListFamilies(home)
	if !reflect.DeepEqual(got, []string{"User Font"}) {
		t.Fatalf("ListFamilies(%q) = %#v, want User Font", home, got)
	}
}

// The app ships as one binary for three platforms, so font discovery must not
// be macOS-only. It was, which left the settings font pickers empty on Windows
// and Linux with nothing to explain why.
func TestSystemFontDirsCoverEveryTargetPlatform(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		if dirs := systemFontDirs(goos); len(dirs) == 0 {
			t.Errorf("systemFontDirs(%q) returned nothing", goos)
		}
	}
	if got := userFontDir("darwin", "/Users/x"); got != "/Users/x/Library/Fonts" {
		t.Errorf("darwin user dir = %q", got)
	}
	if got := userFontDir("linux", "/home/x"); got != "/home/x/.local/share/fonts" {
		t.Errorf("linux user dir = %q", got)
	}
	if got := userFontDir("linux", ""); got != "" {
		t.Errorf("no home should yield no user dir, got %q", got)
	}
}
