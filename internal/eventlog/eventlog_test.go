package eventlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	t := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	return func() time.Time { t = t.Add(time.Second); return t }
}

// The point of this package: after a failure, someone can read what happened.
// Every record must carry when, what, and the build it came from — a report
// that cannot be tied to a version cannot be reproduced.
func TestRecordsAreReadableAndCarryTheBuild(t *testing.T) {
	dir := t.TempDir()
	log := New(dir, "0.4.0", fixedClock())

	log.Record("document.save.failed", map[string]string{"path": "/tmp/a.md", "error": "disk full"})

	data, err := os.ReadFile(filepath.Join(dir, "events.log"))
	if err != nil {
		t.Fatalf("no log was written: %v", err)
	}
	line := string(data)
	for _, want := range []string{"2026-08-08T12:00:01Z", "document.save.failed", "/tmp/a.md", "disk full", "0.4.0"} {
		if !strings.Contains(line, want) {
			t.Errorf("record is missing %q: %s", want, line)
		}
	}
}

// A log that grows without bound fills a user's disk, and one that is trimmed
// by deleting the file loses the failure that was being investigated. Keep the
// most recent entries and drop the oldest.
func TestOldEntriesAreDroppedAndRecentOnesKept(t *testing.T) {
	dir := t.TempDir()
	log := New(dir, "0.4.0", fixedClock())
	log.maxEntries = 10

	for i := 0; i < 50; i++ {
		log.Record("tick", map[string]string{"n": string(rune('a' + i%26))})
	}
	log.Record("the.last.event", nil)

	data, err := os.ReadFile(filepath.Join(dir, "events.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 11 {
		t.Errorf("log grew unbounded: %d lines", len(lines))
	}
	if !strings.Contains(lines[len(lines)-1], "the.last.event") {
		t.Errorf("the most recent event was not kept: %q", lines[len(lines)-1])
	}
}

// Recording is diagnostic. It must never be able to break the thing it is
// observing, so an unwritable directory is swallowed rather than propagated.
func TestRecordingNeverFailsTheCaller(t *testing.T) {
	log := New("/nonexistent/path/that/cannot/be/created\x00", "0.4.0", fixedClock())
	log.Record("should.not.panic", map[string]string{"k": "v"})
}

// Values are written as JSON, so a path containing a quote or newline cannot
// forge a second record or corrupt the line it sits on.
func TestValuesCannotForgeARecord(t *testing.T) {
	dir := t.TempDir()
	log := New(dir, "0.4.0", fixedClock())

	log.Record("open", map[string]string{"path": "a\"b\nlevel=FORGED event=admin.granted"})

	data, _ := os.ReadFile(filepath.Join(dir, "events.log"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Errorf("a value broke the record onto %d lines: %q", len(lines), data)
	}
}
