package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"dr-markdown/internal/eventlog"
)

func appWithEventLogIn(t *testing.T, dir string) (*App, *fakeNative) {
	t.Helper()
	native := &fakeNative{}
	app := newAppWithDependencies(appDependencies{
		events: eventlog.New(dir, appVersion, time.Now),
		native: native,
	})
	app.ctx = context.Background()
	return app, native
}

// Wails recovers panics in bound method dispatch and turns them into a promise
// rejection, so the app already survives one — silently. Nothing reaches the
// event trail, and the Wails logger it does reach writes to a stream no
// packaged-app user can read. A user who reports "it stopped saving" therefore
// hands over no recorded state at all.
//
// Three things must happen, and the third is why this is not just a Record
// call: the panic has to keep travelling. Recovering it here would return the
// method's zero values, so SaveDocument would hand the frontend a nil error and
// the frontend would report a save that never happened.
func TestPanicIsRecordedAndShownAndStillPropagates(t *testing.T) {
	dir := t.TempDir()
	app, native := appWithEventLogIn(t, dir)

	propagated := func() (p any) {
		defer func() { p = recover() }()
		defer app.reportPanic("SaveDocument")
		panic("boom")
	}()

	if propagated == nil {
		t.Fatal("the panic was swallowed: the bound method would return its zero values, " +
			"so the frontend would be told a failed save succeeded")
	}

	data, err := os.ReadFile(filepath.Join(dir, "events.log"))
	if err != nil {
		t.Fatalf("the panic left no recorded state: %v", err)
	}
	line := string(data)
	for _, want := range []string{"panic", "SaveDocument", "boom", appVersion} {
		if !strings.Contains(line, want) {
			t.Errorf("the record is missing %q, so the report cannot be root-caused: %s", want, line)
		}
	}
	if !strings.Contains(line, "crash_test.go") {
		t.Errorf("the record carries no stack, so it names no source line: %s", line)
	}

	if native.errorTitle == "" {
		t.Error("the user was not told. The app keeps running after Wails recovers, " +
			"so an untold user edits on in a state the app no longer understands")
	}
	if !strings.Contains(native.errorMessage, "SaveDocument") {
		t.Errorf("the dialog does not name the operation that failed: %q", native.errorMessage)
	}
}

// A guard that fires on the happy path would write a panic record for every
// successful save, and the trail's value is that its entries mean something.
func TestAnOperationThatDoesNotPanicRecordsNothing(t *testing.T) {
	dir := t.TempDir()
	app, native := appWithEventLogIn(t, dir)

	func() {
		defer app.reportPanic("SaveDocument")
	}()

	if _, err := os.Stat(filepath.Join(dir, "events.log")); !os.IsNotExist(err) {
		data, _ := os.ReadFile(filepath.Join(dir, "events.log"))
		t.Errorf("a successful operation wrote to the trail: %s", data)
	}
	if native.errorTitle != "" {
		t.Errorf("a successful operation showed an error dialog: %q", native.errorTitle)
	}
}

// Every method the frontend can call must carry the guard, and this test exists
// because there is no way to install it centrally. Wails calls bound methods by
// reflection, so there is no wrapper to hang it on; ErrorFormatter is not a seam
// either, because a panic unwinds past the line that would call it
// (internal/frontend/dispatcher/calls.go builds the callback message only after
// the call returns normally).
//
// Partial coverage would be worse than none. The trail would carry panics from
// the guarded methods and silence from the rest, and silence would read as
// "this method did not panic".
func TestEveryBoundMethodReportsItsPanics(t *testing.T) {
	src, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	// Prove the instrument before trusting what it does not find. The first
	// version of this detector required a newline after the opening brace and so
	// skipped UpdateContent, which is written on one line — it reported a clean
	// method by never looking at it. Counting signatures separately from bodies
	// makes that class of miss fail loudly instead of passing quietly.
	signatures := regexp.MustCompile(`func \(a \*App\) [A-Z]\w*\(`).FindAllString(string(src), -1)
	bodies := regexp.MustCompile(`func \(a \*App\) ([A-Z]\w*)\([^)]*\)[^{]*\{`).
		FindAllStringSubmatchIndex(string(src), -1)
	if len(signatures) == 0 {
		t.Fatal("found no exported App methods; the detector is broken, not the code")
	}
	if len(bodies) != len(signatures) {
		t.Fatalf("the detector matched %d method bodies but %d signatures; it is skipping methods, "+
			"so a clean result would mean nothing", len(bodies), len(signatures))
	}

	var unguarded []string
	for _, m := range bodies {
		name := string(src[m[2]:m[3]])
		body := strings.TrimLeft(string(src[m[1]:]), " \t\n")
		if !strings.HasPrefix(body, "defer a.reportPanic("+strconv.Quote(name)+")") {
			unguarded = append(unguarded, name)
		}
	}
	if len(unguarded) > 0 {
		sort.Strings(unguarded)
		t.Errorf("these bound methods do not report a panic, so one in them reaches nobody:\n  %s",
			strings.Join(unguarded, "\n  "))
	}
}

// The Wails lifecycle callbacks are not bound methods, so the dispatcher's
// recover never sees them. A panic in one of these unwinds out of the goroutine
// that called it and takes the process with it — the crash the app has always
// had, with nothing written down. main.go registers each of these by name.
func TestLifecycleCallbacksReportTheirPanics(t *testing.T) {
	src, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"startup", "beforeClose", "openFileFromOS"} {
		m := regexp.MustCompile(`func \(a \*App\) ` + name + `\([^)]*\)[^{]*\{`).
			FindStringIndex(string(src))
		if m == nil {
			t.Fatalf("%s is registered in main.go but no longer exists here; "+
				"the detector is broken, not the code", name)
		}
		body := strings.TrimLeft(string(src[m[1]:]), " \t\n")
		if !strings.HasPrefix(body, "defer a.reportPanic("+strconv.Quote(name)+")") {
			t.Errorf("%s does not report a panic, and a panic there kills the process silently", name)
		}
	}
}

// The dialog must not storm. UpdateContent is pushed debounced on every edit, so
// a panic there repeats for as long as the user types; a blocking native dialog
// per tick would make the app unusable, which is worse than the silently dead
// call it replaced.
//
// The trail keeps every panic, because that is what root-causing needs. The
// dialog is capped instead, and the cap is one per session rather than one per
// operation: its instruction is "restart", and that does not improve on
// repetition. This mirrors the capping already applied to refused link schemes,
// for the same reason — a failure that repeats must not drown out everything
// else.
func TestRepeatedPanicsRecordEveryTimeButShowOneDialog(t *testing.T) {
	dir := t.TempDir()
	app, native := appWithEventLogIn(t, dir)

	for _, op := range []string{"UpdateContent", "UpdateContent", "SaveDocument"} {
		func() {
			defer func() { _ = recover() }()
			defer app.reportPanic(op)
			panic("boom")
		}()
	}

	data, err := os.ReadFile(filepath.Join(dir, "events.log"))
	if err != nil {
		t.Fatalf("no panics were recorded: %v", err)
	}
	if got := strings.Count(string(data), `"event":"panic"`); got != 3 {
		t.Errorf("the trail must keep every panic, got %d of 3:\n%s", got, data)
	}
	if native.errorCount != 1 {
		t.Errorf("showed %d dialogs; a repeating panic would block the app behind a modal storm",
			native.errorCount)
	}
}
