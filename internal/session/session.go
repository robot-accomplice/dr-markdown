// Package session owns the open-document session: which tabs exist, which has
// focus, which have unsaved changes, and what this app last read from or wrote
// to each file on disk.
//
// It exists because that state used to live on the Wails binding surface as
// loose fields behind a shared mutex, with its rules recorded only in comments
// that every reader had to remember to honour. Those rules are not stylistic —
// getting one wrong destroyed a document the user had not touched.
package session

import "sync"

// Document is one editor tab as the frontend reports it. The json tags are the
// wire contract with the webview and must not change.
type Document struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Dirty   bool   `json:"dirty"`
	Active  bool   `json:"active"`
}

// Session is safe for concurrent use. Its zero value is ready.
type Session struct {
	mu   sync.Mutex
	docs []Document
	// unsyncedDirty covers a frontend that reported dirty before ever syncing
	// its documents. It forces a prompt; it never names a write target.
	unsyncedDirty bool
}

// Sync replaces the known tabs with what the frontend reported.
//
// The slice is copied: keeping the caller's backing array would let the next
// push from the frontend mutate what the close guard is about to read.
func (s *Session) Sync(docs []Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = append(s.docs[:0], docs...)
}

// Active returns the focused tab, or the first, or a zero value.
//
// The zero value names no file, deliberately. Go must never infer a write
// target from ambient state — that is what destroyed documents.
func (s *Session) Active() Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.docs {
		if d.Active {
			return d
		}
	}
	if len(s.docs) > 0 {
		return s.docs[0]
	}
	return Document{}
}

// Dirty returns a copy of every tab with unsaved changes.
func (s *Session) Dirty() []Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Document
	for _, d := range s.docs {
		if d.Dirty {
			out = append(out, d)
		}
	}
	return out
}

// SetDirty records the frontend's dirty state for the active tab.
//
// With no synced documents the flag is kept on its own rather than invented
// against the last opened path: not knowing which file is dirty must lead to
// asking the user, never to writing a guess.
func (s *Session) SetDirty(dirty bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unsyncedDirty = dirty && len(s.docs) == 0
	for i := range s.docs {
		if s.docs[i].Active {
			s.docs[i].Dirty = dirty
		}
	}
}

// HasUnsyncedDirty reports a dirty frontend that never named its documents.
//
// The tab count is re-checked at read time rather than trusted from when the
// flag was set: a frontend can report dirty and only then sync its documents,
// and once they are known the answer is no longer "we have no idea which file".
func (s *Session) HasUnsyncedDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unsyncedDirty && len(s.docs) == 0
}

// UpdateActiveContent stores the latest markdown for the active tab, pushed
// debounced by the frontend, so the close guard can save without a round trip.
func (s *Session) UpdateActiveContent(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.docs {
		if s.docs[i].Active {
			s.docs[i].Content = content
		}
	}
}

// snapshotForTest returns a copy of every tab. Test-only: production code asks
// specific questions (Active, Dirty) rather than reading the whole list.
func (s *Session) snapshotForTest() []Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Document(nil), s.docs...)
}

// AdoptPath records that a tab now lives at path with this content, and is
// clean. It matches the tab already at that path, or the active tab that has no
// path yet — the Save-As case.
//
// Save and open both need exactly this, and both used to carry their own copy
// of the loop. Two copies of a rule about which tab a write lands on is how one
// tab's content reached another tab's file.
func (s *Session) AdoptPath(path, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.docs {
		if s.docs[i].Path == path || (s.docs[i].Active && s.docs[i].Path == "") {
			s.docs[i].Path = path
			s.docs[i].Content = content
			s.docs[i].Dirty = false
		}
	}
}
