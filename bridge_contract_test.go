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
// The generated binding is gitignored, so a stale one is invisible to
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

// Every method the bridge calls on native() must be in the bound surface the
// Objective-C host installs.
//
// TestBridgeOnlyCallsMethodsAppProvides proves the Go side can answer a call;
// it says nothing about the JS object the host builds, which comes from a
// hardcoded NAMES array in host_darwin.m. SetAsDefaultMarkdownHandler was
// added to bridge.js and app.go but not to NAMES, so the call threw a
// TypeError in the real app behind bridge.js's degradation message. This
// compares the bridge against the bound surface itself, so the next method
// added on two of the three surfaces fails here.
func TestBridgeOnlyCallsMethodsTheBoundSurfaceProvides(t *testing.T) {
	bridge, err := os.ReadFile("frontend/dist/src/bridge.js")
	if err != nil {
		t.Fatalf("read bridge: %v", err)
	}
	host, err := os.ReadFile("host_darwin.m")
	if err != nil {
		t.Fatalf("read host_darwin.m: %v", err)
	}

	// The bound surface, parsed out of the ObjC string literals: the region
	// between "const NAMES = [" and "];" holds the names as 'Name' literals.
	namesRegion := regexp.MustCompile(`(?s)const NAMES = \[(.*?)\];`).FindStringSubmatch(string(host))
	if namesRegion == nil {
		t.Fatal("found no NAMES array in host_darwin.m; the detector is broken, not the code")
	}
	bound := map[string]bool{}
	for _, m := range regexp.MustCompile(`'([A-Z]\w*)'`).FindAllStringSubmatch(namesRegion[1], -1) {
		bound[m[1]] = true
	}
	if len(bound) == 0 {
		t.Fatal("the NAMES region held no method names; the detector is broken, not the code")
	}

	// Names the bridge reaches for on the native object — in CODE, not in
	// prose, for the reasons given in TestBridgeOnlyCallsMethodsAppProvides.
	var missing []string
	seen := map[string]bool{}
	code := stripJSComments(string(bridge))
	for _, m := range regexp.MustCompile(`(?:native\(\)|app)\??\.([A-Z]\w*)`).FindAllStringSubmatch(code, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if !bound[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("bridge.js calls methods missing from the bound surface in host_darwin.m: %s\nNAMES provides: %s",
			strings.Join(missing, ", "), strings.Join(keys(bound), ", "))
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
