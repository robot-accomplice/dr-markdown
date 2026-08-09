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
