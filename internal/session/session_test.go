package session

import "testing"

// The session owns the rules that once destroyed documents. They were enforced
// by every reader remembering to honour them; here they are stated once and
// tested directly, with no App and no ports.

func TestActiveReturnsTheFocusedTab(t *testing.T) {
	var s Session
	s.Sync([]Document{
		{Path: "/a.md", Content: "A"},
		{Path: "/b.md", Content: "B", Active: true},
	})
	if got := s.Active(); got.Path != "/b.md" {
		t.Errorf("Active() = %q, want /b.md", got.Path)
	}
}

// A frontend that has not marked any tab active must still yield a usable
// answer rather than a zero value that names no file.
func TestActiveFallsBackToTheFirstTab(t *testing.T) {
	var s Session
	s.Sync([]Document{{Path: "/a.md"}, {Path: "/b.md"}})
	if got := s.Active(); got.Path != "/a.md" {
		t.Errorf("Active() = %q, want /a.md", got.Path)
	}
}

// With no tabs at all the answer must be a zero Document — never a guess.
func TestActiveWithNoTabsNamesNoFile(t *testing.T) {
	var s Session
	if got := s.Active(); got.Path != "" {
		t.Errorf("Active() on an empty session named %q; it must name no file", got.Path)
	}
}

func TestDirtyReturnsOnlyUnsavedTabs(t *testing.T) {
	var s Session
	s.Sync([]Document{
		{Path: "/a.md", Dirty: true},
		{Path: "/b.md"},
		{Path: "/c.md", Dirty: true},
	})
	got := s.Dirty()
	if len(got) != 2 || got[0].Path != "/a.md" || got[1].Path != "/c.md" {
		t.Errorf("Dirty() = %+v, want /a.md and /c.md", got)
	}
}

// Sync must copy. Handing the caller's slice back would let the frontend's next
// push mutate what the close guard is about to read.
func TestSyncCopiesTheCallersSlice(t *testing.T) {
	var s Session
	docs := []Document{{Path: "/a.md", Dirty: true}}
	s.Sync(docs)
	docs[0].Dirty = false
	if len(s.Dirty()) != 1 {
		t.Error("mutating the caller's slice after Sync changed the session's view")
	}
}
