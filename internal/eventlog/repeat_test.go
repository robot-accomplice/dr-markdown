package eventlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func read(t *testing.T, dir string) []map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "events.log"))
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	var out []map[string]string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]string
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("record is not JSON: %v\n%s", err, line)
		}
		out = append(out, rec)
	}
	return out
}

func fixed() func() time.Time {
	return func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) }
}

// The whole point of #63: a repeating panic must not evict the trail that
// explains it.
//
// Before this, App.reportPanic recorded every panic with a full stack and
// nothing capped recording, while UpdateContent is pushed debounced on every
// edit — so a panic there produced a record per tick for as long as the user
// typed. trim keeps the NEWEST 2000, so the first panic and everything leading
// to it were evicted and 2000 copies of the last one were kept. That inverts
// what root-cause analysis needs.
//
// This test writes the events that lead up to a failure, then floods, and
// requires the LEAD-UP to still be there.
func TestARepeatingEventDoesNotEvictWhatLedToIt(t *testing.T) {
	dir := t.TempDir()
	log := New(dir, "1.6.1", fixed())

	log.Record("app.started", map[string]string{"mode": "wysiwyg"})
	log.Record("document.opened", map[string]string{"path": "/tmp/thesis.md"})
	log.Record("image.inserted", map[string]string{"path": "/tmp/figure.png"})

	// Then the same panic, once per debounce tick, far past the 2000-line bound.
	for i := 0; i < 5000; i++ {
		log.Record("panic", map[string]string{
			"operation": "UpdateContent",
			"message":   "index out of range",
			"stack":     "goroutine 1 [running]:\n...",
		})
	}

	records := read(t, dir)
	t.Logf("5000 repeats produced %d records in total", len(records))

	for _, want := range []string{"app.started", "document.opened", "image.inserted"} {
		found := false
		for _, r := range records {
			if r["event"] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q was evicted by the repeating panic: the events that led to a failure "+
				"are exactly what root-cause analysis needs, and they are gone", want)
		}
	}

	if len(records) >= defaultMaxEntries {
		t.Errorf("the log filled to %d records, so trimming was still reached: folding did not "+
			"bound the flood", len(records))
	}
}

// Frequency must survive, or folding trades one blind spot for another: "it
// panicked five times" and "it panicked five thousand times" are different
// diagnoses.
func TestAFoldedRecordCarriesHowManyHappened(t *testing.T) {
	dir := t.TempDir()
	log := New(dir, "1.6.1", fixed())
	for i := 0; i < 4000; i++ {
		log.Record("panic", map[string]string{"operation": "UpdateContent", "message": "boom"})
	}

	var milestones []string
	records := read(t, dir)
	for _, r := range records {
		if n := r["repeats"]; n != "" {
			milestones = append(milestones, n)
		}
	}
	t.Logf("%d records for 4000 occurrences; folded records report %v", len(records), milestones)

	if len(milestones) == 0 {
		t.Fatal("no record carries a repeat count: 4000 occurrences are indistinguishable from 5")
	}
	if last := milestones[len(milestones)-1]; last != "1000" {
		t.Errorf("the last folded record reports %q; expected the 1000 milestone, so the scale "+
			"of the repetition is legible rather than merely its existence", last)
	}
}

// Folding must not merge events that are genuinely different, or a trail
// becomes a lie in the other direction.
func TestDistinctEventsAreNotFoldedTogether(t *testing.T) {
	dir := t.TempDir()
	log := New(dir, "1.6.1", fixed())
	for i := 0; i < 20; i++ {
		log.Record("panic", map[string]string{"operation": "UpdateContent", "message": "a"})
		log.Record("panic", map[string]string{"operation": "SaveDocument", "message": "b"})
	}
	ops := map[string]int{}
	for _, r := range read(t, dir) {
		ops[r["operation"]]++
	}
	for _, op := range []string{"UpdateContent", "SaveDocument"} {
		if ops[op] == 0 {
			t.Errorf("%q was folded away entirely: two different panics must not share a "+
				"signature", op)
		}
	}
}

// A stack differs between two occurrences of the same panic. If it counted
// towards the signature, nothing would ever fold.
func TestAChangingStackStillFolds(t *testing.T) {
	dir := t.TempDir()
	log := New(dir, "1.6.1", fixed())
	for i := 0; i < 500; i++ {
		log.Record("panic", map[string]string{
			"operation": "UpdateContent",
			"message":   "boom",
			"stack":     "goroutine " + strconv.Itoa(i) + " [running]",
		})
	}
	if n := len(read(t, dir)); n > 20 {
		t.Errorf("500 occurrences with differing stacks produced %d records: the stack is "+
			"counting towards the signature, so nothing folds", n)
	}
}
