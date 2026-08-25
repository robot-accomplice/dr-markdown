package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostFiles are the only Go files permitted to reach the operating system
// directly. Keep this list short: every entry is a file a port to another
// platform has to rewrite.
var hostFiles = map[string]bool{
	"host_darwin.go":           true,
	"host_lifecycle_darwin.go": true,
	"harness_darwin.go":        true,
	"walk_darwin.go":           true,
	"modal_check_darwin.go":    true,
	"host_unsupported.go":      true,
}

// The point of the boundary is that swapping the host is a leaf change. That is
// only true while the host stays confined, and "confined" is a claim that decays
// silently — one import in app.go and it is false again with nothing to notice.
//
// This is also the measurement the A/B/C decision rests on: whatever these files
// weigh is what a replacement costs to write.
func TestOnlyTheHostFilesReachTheOperatingSystem(t *testing.T) {
	// cgo is the marker now that no framework is imported. Reaching AppKit means
	// importing "C", so a file that does is by definition part of the host.
	//
	// Split so this file does not contain the literal it searches for. Spelling
	// it whole made the previous detector match ITSELF on its first run, and
	// exempting this file instead would have dropped every other _test.go from
	// scope — which is exactly where a stray host dependency slips in.
	needle := "import " + `"C"`

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
		t.Errorf("these files reach the operating system from outside the boundary:\n  %s\n"+
			"Move the dependency behind hostPort, or add the file to hostFiles and accept "+
			"that a port to another platform must rewrite it.", strings.Join(offenders, "\n  "))
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
