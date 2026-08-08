package fonts

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// fontDirs is the resolved system font directory list. It stays a package
// variable so tests can isolate discovery from the host's real fonts.
var fontDirs = systemFontDirs(runtime.GOOS)

// systemFontDirs returns the OS font directories. This ships as one binary for
// macOS, Windows and Linux; the list was macOS-only, so the settings font
// pickers came up empty everywhere else with no error to explain it.
func systemFontDirs(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"/System/Library/Fonts", "/Library/Fonts"}
	case "windows":
		dirs := []string{`C:\Windows\Fonts`}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
		}
		return dirs
	default: // linux and other unix
		return []string{"/usr/share/fonts", "/usr/local/share/fonts"}
	}
}

// userFontDir returns the per-user font directory for goos, relative to home.
func userFontDir(goos, home string) string {
	if home == "" {
		return ""
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Fonts")
	case "windows":
		return "" // covered by LOCALAPPDATA above
	default:
		return filepath.Join(home, ".local", "share", "fonts")
	}
}

// ListFamilies returns installed font family names discovered from macOS font
// files. It intentionally reports family-level names rather than individual
// style faces where the filename makes that distinction clear.
func ListFamilies(home string) []string {
	dirs := append([]string{}, fontDirs...)
	if userDir := userFontDir(runtime.GOOS, home); userDir != "" {
		dirs = append(dirs, userDir)
	}

	names := map[string]bool{}
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if name := familyName(path); name != "" {
				names[name] = true
			}
			return nil
		})
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func familyName(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
		return ""
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.Join(strings.Fields(name), " ")
	return trimStyleSuffix(name)
}

func trimStyleSuffix(name string) string {
	styleWords := map[string]bool{
		"black": true, "bold": true, "book": true, "condensed": true,
		"demi": true, "heavy": true, "italic": true, "light": true,
		"medium": true, "oblique": true, "regular": true, "semibold": true,
		"thin": true,
	}
	parts := strings.Fields(name)
	for len(parts) > 1 && styleWords[strings.ToLower(parts[len(parts)-1])] {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, " ")
}
