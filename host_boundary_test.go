package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostFiles are the only Go files permitted to name the host framework. Keep
// this list short: every entry is a file a replacement has to rewrite.
var hostFiles = map[string]bool{"host_wails.go": true}

// The point of the boundary is that swapping the host is a leaf change. That is
// only true while the host stays confined, and "confined" is a claim that decays
// silently — one import in app.go and it is false again with nothing to notice.
//
// This is also the measurement the A/B/C decision rests on: whatever these files
// weigh is what a replacement costs to write.
func TestOnlyTheHostFilesNameWails(t *testing.T) {
	// Split so this file does not contain the literal string it searches for.
	// Spelling it whole made the detector match itself on its first run — and
	// exempting this file instead would have dropped every other _test.go from
	// scope, which is where a stray host import is most likely to slip in.
	needle := "wailsapp" + "/wails"

	var offenders []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// tools/ are build-time programs, not the shipped application.
			if info.Name() == "tools" || info.Name() == "build" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || hostFiles[filepath.Base(path)] {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), needle) {
			offenders = append(offenders, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("these files import the host framework from outside the boundary:\n  %s\n"+
			"Move the dependency behind hostPort, or add the file to hostFiles and accept "+
			"that a replacement must rewrite it.", strings.Join(offenders, "\n  "))
	}
}

// Prove the detector looks at anything at all. A walk that silently matches no
// files reports a clean boundary by never checking one — the same failure as a
// coverage gate that skips its target.
func TestTheBoundaryDetectorActuallyReadsGoFiles(t *testing.T) {
	var seen int
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".go") {
			seen++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 10 {
		t.Fatalf("the walk found only %d Go files; the detector is broken, not the code", seen)
	}
}
