// Package eventlog records what the application did, so a failure reported
// later can be root-caused instead of guessed at.
//
// Until this existed, nothing survived a session: frontend diagnostics were
// console warnings with no devtools in a production build, and Go errors
// surfaced only as a modal dialog that vanished when dismissed. A user
// reporting "it deleted my images" or "it crashed when I quit" could not tell
// you which build they ran, and there was no recorded state to replay. An
// adversarial review named this as the single issue that makes every other
// silent failure unattributable.
//
// The design constraints follow from that:
//
//   - It is diagnostic, so it must never break what it observes. Every failure
//     to record is swallowed.
//   - It must be bounded, or it fills the user's disk.
//   - Trimming keeps the NEWEST entries. Truncating the file instead would
//     discard the failure being investigated.
//   - A REPEATING event is folded, or keeping the newest becomes the defect.
//     See collapseRepeats below: an event that recurs on a debounced path can
//     write a record per tick, and 2000 copies of the last one evict the first
//     one and everything that led to it — inverting what root-cause analysis
//     needs out of a trail that keeps the newest.
//   - Values are JSON-encoded, so a document path containing a quote or a
//     newline cannot forge a record or corrupt the line it sits on. Paths come
//     from opened documents, which are untrusted input.
package eventlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultMaxEntries = 2000

// Repetition folding. GitHub #63: App.reportPanic records every panic with a
// full stack and nothing caps recording, while UpdateContent is pushed debounced
// on every edit — so a panic there produces a record per tick for as long as the
// user types. Trimming keeps the newest, so the FIRST panic and the events that
// led to it are evicted and 2000 copies of the last one are kept.
//
// The same shape is already solved for refused link schemes, which are recorded
// once per distinct href and capped, because the log is trimmed and a document
// the app has judged hostile must not be able to erase the rest of the trail. A
// document that reliably panics a bound method has that power by another route.
//
// Frequency is kept rather than discarded: the first few occurrences are
// recorded in full, and after that only at decade milestones, carrying the count
// so far. That is O(log n) records for n occurrences — enough to say "this
// happened 4000 times" without the 4000 records that would evict everything
// else.
const (
	// Occurrences of one signature recorded in full before folding starts.
	repeatBurst = 5
	// Distinct signatures tracked at once. Past this, events are recorded
	// unfolded: a flood spread across thousands of DISTINCT signatures is a
	// different problem from the reported one, and suppressing events to guess
	// at it would lose records that are not repeats.
	maxTrackedSignatures = 64
)

// Fields that differ between two occurrences of the SAME event and must not make
// them look distinct. A stack differs by goroutine state; a time always differs.
var volatileFields = map[string]bool{"stack": true, "time": true}

// Log appends structured records to a bounded file.
type Log struct {
	mu         sync.Mutex
	dir        string
	version    string
	now        func() time.Time
	maxEntries int
	// seen counts occurrences per signature, for repetition folding.
	seen map[string]int
}

// New returns a log writing to dir/events.log, stamping each record with the
// build version so a report can be tied to what actually ran.
func New(dir, version string, now func() time.Time) *Log {
	if now == nil {
		now = time.Now
	}
	return &Log{dir: dir, version: version, now: now, maxEntries: defaultMaxEntries,
		seen: map[string]int{}}
}

// collapseRepeats reports whether this occurrence should be written, and how
// many have been seen, so a folded record can say so.
//
// Called with l.mu held.
func (l *Log) collapseRepeats(event string, fields map[string]string) (write bool, count int) {
	sig := signature(event, fields)
	if _, tracked := l.seen[sig]; !tracked && len(l.seen) >= maxTrackedSignatures {
		// Not tracking any more signatures. Record unfolded rather than
		// suppress: losing a record that is not a repeat would be worse than
		// the flood this guards against.
		return true, 0
	}
	l.seen[sig]++
	n := l.seen[sig]
	if n <= repeatBurst {
		return true, n
	}
	// After the burst, only decade milestones: 10, 100, 1000, ...
	for m := 10; m <= n; m *= 10 {
		if n == m {
			return true, n
		}
	}
	return false, n
}

// signature identifies two records as occurrences of the same event. Volatile
// fields are excluded, or every occurrence looks distinct and nothing folds.
func signature(event string, fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if !volatileFields[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(event)
	for _, k := range keys {
		b.WriteByte(0x1f) // unit separator: cannot occur in a JSON string value
		b.WriteString(k)
		b.WriteByte(0x1f)
		b.WriteString(fields[k])
	}
	return b.String()
}

// Record appends one event. Errors are deliberately ignored: a diagnostic
// facility that can fail a save is worse than no diagnostic facility.
func (l *Log) Record(event string, fields map[string]string) {
	if l == nil {
		return
	}
	record := map[string]string{
		"time":    l.now().UTC().Format(time.RFC3339),
		"event":   event,
		"version": l.version,
	}
	for k, v := range fields {
		if k == "time" || k == "event" || k == "version" {
			continue // never let a caller overwrite the envelope
		}
		record[k] = v
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Fold repeats BEFORE writing, so a repeating event cannot reach the file
	// often enough to trim the rest of the trail away.
	write, count := l.collapseRepeats(event, fields)
	if !write {
		return
	}
	if count > repeatBurst {
		// A folded record says how many have happened, so frequency survives
		// even though the individual records did not.
		record["repeats"] = strconv.Itoa(count)
		line, err = json.Marshal(record)
		if err != nil {
			return
		}
	}

	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return
	}
	path := filepath.Join(l.dir, "events.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
	l.trim(path)
}

// trim keeps the most recent maxEntries lines.
func (l *Log) trim(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) <= l.maxEntries {
		return
	}
	kept := strings.Join(lines[len(lines)-l.maxEntries:], "\n") + "\n"
	_ = os.WriteFile(path, []byte(kept), 0o600)
}
