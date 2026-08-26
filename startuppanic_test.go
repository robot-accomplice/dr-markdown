package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dr-markdown/internal/eventlog"
)

func startupRecords(t *testing.T, dir string) []map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "events.log"))
	if err != nil {
		return nil
	}
	var out []map[string]string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]string
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("record is not JSON: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

// A panic before the application is running must leave a trail.
//
// #62. Measured against the startup path before this existed, by panicking at
// each point and checking whether anything reached events.log:
//
//	inside NewApp, before the log is built   nothing — no file at all
//	after NewApp, before host.Run            nothing, though the log existed
//	inside app.startup                       recorded
//
// The second line is why the issue's own framing was wrong: the boundary was
// never whether the App exists, it was whether the call happens to be wrapped
// by reportPanic — and main is not.
func TestAPanicBeforeTheApplicationIsRunningLeavesATrail(t *testing.T) {
	dir := t.TempDir()
	log := eventlog.New(dir, "1.6.1", time.Now)

	var repanicked any
	func() {
		defer func() { repanicked = recover() }()
		defer recordStartupPanic(log)
		panic("preferences store exploded")
	}()

	if repanicked == nil {
		t.Fatal("the panic was swallowed. A failure during startup means the application " +
			"cannot run, and continuing into a half-built state trades a visible crash for " +
			"an invisible one — the point is the record, not the rescue")
	}

	records := startupRecords(t, dir)
	if len(records) == 0 {
		t.Fatal("nothing was recorded: this is the defect, not a variation of it")
	}
	r := records[0]
	if r["event"] != "panic" {
		t.Errorf("recorded %q, want a panic record", r["event"])
	}
	if !strings.Contains(r["message"], "preferences store exploded") {
		t.Errorf("the record does not carry what happened: %q", r["message"])
	}
	if r["stack"] == "" {
		t.Error("no stack recorded, so the trail says a panic happened but not where")
	}
	// The phase distinguishes this from a panic in a bound method, which is a
	// different kind of failure with a different investigation.
	if r["phase"] == "" {
		t.Error("no phase recorded: a startup panic and a runtime panic read identically")
	}
}

// The common path must stay silent. A guard that records on every clean start
// would fill the trail with non-events.
func TestACleanStartupRecordsNothing(t *testing.T) {
	dir := t.TempDir()
	log := eventlog.New(dir, "1.6.1", time.Now)

	func() {
		defer recordStartupPanic(log)
	}()

	if n := len(startupRecords(t, dir)); n != 0 {
		t.Errorf("a startup with no panic wrote %d record(s)", n)
	}
}

// The trail must be openable without the App, or it cannot cover the App's own
// construction — which is the whole point.
func TestTheTrailOpensWithoutTheApplication(t *testing.T) {
	if NewEventLog() == nil {
		t.Fatal("NewEventLog returned nil, so nothing could be recorded before NewApp")
	}
}
