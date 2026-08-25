package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every Go method the frontend bridge calls must exist on App.
//
// The generated Wails binding is gitignored, so a stale one is invisible to
// review: the copy on the build machine lacked SyncDocuments entirely, the
// bridge optional-called it into a silent no-op, and Go was left with no
// documents at all. This compares the two tracked sources instead, so a
// binding the frontend expects and Go does not provide fails here rather than
// at a user's desk.
func TestBridgeOnlyCallsMethodsAppProvides(t *testing.T) {
	bridge, err := os.ReadFile("frontend/dist/src/bridge.js")
	if err != nil {
		t.Fatalf("read bridge: %v", err)
	}
	appSrc, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatalf("read app.go: %v", err)
	}

	// Exported methods on *App.
	provided := map[string]bool{}
	for _, m := range regexp.MustCompile(`func \(a \*App\) ([A-Z]\w*)\(`).FindAllStringSubmatch(string(appSrc), -1) {
		provided[m[1]] = true
	}
	if len(provided) == 0 {
		t.Fatal("found no exported App methods; the detector is broken, not the code")
	}

	// Names the bridge reaches for on the native object — in CODE, not in prose.
	//
	// Comments are stripped first. A comment that mentions the call shape
	// invents a method out of thin air, and one that contains a real call can
	// hide a missing method behind a line nothing executes. This test caught its
	// own documentation the day the shape was renamed.
	var missing []string
	seen := map[string]bool{}
	code := stripJSComments(string(bridge))
	for _, m := range regexp.MustCompile(`(?:native\(\)|app)\??\.([A-Z]\w*)`).FindAllStringSubmatch(code, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if !provided[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("bridge.js calls App methods that do not exist: %s\nApp provides: %s",
			strings.Join(missing, ", "), strings.Join(keys(provided), ", "))
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// stripJSComments removes // line comments and /* */ blocks.
//
// Deliberately simple: it does not understand strings, so a comment marker
// inside a string literal would be treated as a comment. bridge.js contains no
// such string, and a detector that needs a JavaScript parser to answer "which
// methods does this file call" has outgrown being a regex either way.
func stripJSComments(src string) string {
	src = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
	return regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(src, "")
}
