package fonts

import (
	"os"
	"path/filepath"
	"reflect"
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
	userFonts := filepath.Join(home, "Library", "Fonts")
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
