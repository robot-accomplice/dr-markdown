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
//   - Values are JSON-encoded, so a document path containing a quote or a
//     newline cannot forge a record or corrupt the line it sits on. Paths come
//     from opened documents, which are untrusted input.
package eventlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultMaxEntries = 2000

// Log appends structured records to a bounded file.
type Log struct {
	mu         sync.Mutex
	dir        string
	version    string
	now        func() time.Time
	maxEntries int
}

// New returns a log writing to dir/events.log, stamping each record with the
// build version so a report can be tied to what actually ran.
func New(dir, version string, now func() time.Time) *Log {
	if now == nil {
		now = time.Now
	}
	return &Log{dir: dir, version: version, now: now, maxEntries: defaultMaxEntries}
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
