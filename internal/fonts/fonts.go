package fonts

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var fontDirs = []string{
	"/System/Library/Fonts",
	"/Library/Fonts",
}

// ListFamilies returns installed font family names discovered from macOS font
// files. It intentionally reports family-level names rather than individual
// style faces where the filename makes that distinction clear.
func ListFamilies(home string) []string {
	dirs := append([]string{}, fontDirs...)
	if home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
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
