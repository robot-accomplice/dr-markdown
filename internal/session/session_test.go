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

// A frontend that reports dirty before it has ever synced its tabs must force a
// prompt without naming a file. Not knowing WHICH document is dirty has to lead
// to asking the user, never to writing a guess.
func TestDirtyWithNoSyncedTabsForcesAPromptWithoutNamingAFile(t *testing.T) {
	var s Session
	s.SetDirty(true)
	if !s.HasUnsyncedDirty() {
		t.Error("dirty reported before any sync must be remembered")
	}
	if len(s.Dirty()) != 0 {
		t.Error("it must not invent a dirty document")
	}
	if got := s.Active(); got.Path != "" {
		t.Errorf("it must not name a file, got %q", got.Path)
	}
}

// Once tabs exist, the flag belongs to the active tab and the standalone flag
// must clear — otherwise a stale unsynced flag outlives the condition.
func TestDirtyWithSyncedTabsMarksTheActiveTab(t *testing.T) {
	var s Session
	s.Sync([]Document{{Path: "/a.md"}, {Path: "/b.md", Active: true}})
	s.SetDirty(true)
	if s.HasUnsyncedDirty() {
		t.Error("the unsynced flag must not be set once tabs are known")
	}
	dirty := s.Dirty()
	if len(dirty) != 1 || dirty[0].Path != "/b.md" {
		t.Errorf("Dirty() = %+v, want only /b.md", dirty)
	}
}

func TestUpdateActiveContentTargetsOnlyTheActiveTab(t *testing.T) {
	var s Session
	s.Sync([]Document{{Path: "/a.md", Content: "A"}, {Path: "/b.md", Content: "B", Active: true}})
	s.UpdateActiveContent("edited")
	if got := s.Active(); got.Content != "edited" {
		t.Errorf("active content = %q, want edited", got.Content)
	}
	for _, d := range s.snapshotForTest() {
		if d.Path == "/a.md" && d.Content != "A" {
			t.Errorf("the inactive tab's content was overwritten: %q", d.Content)
		}
	}
}

// The baseline is what the app last READ FROM or WROTE TO a file. Comparing
// against what was first opened instead would make every second save look like
// an external edit, and a prompt users learn to click through protects nothing.
func TestBaselineTracksTheLastReadOrWrite(t *testing.T) {
	var s Session
	if _, known := s.BaselineFor("/a.md"); known {
		t.Error("a path the app has never touched must have no baseline")
	}
	s.RememberOnDisk("/a.md", "first")
	if got, known := s.BaselineFor("/a.md"); !known || got != "first" {
		t.Errorf("BaselineFor = %q, %v; want first, true", got, known)
	}
	s.RememberOnDisk("/a.md", "second")
	if got, _ := s.BaselineFor("/a.md"); got != "second" {
		t.Errorf("BaselineFor = %q; a later write must replace the baseline", got)
	}
}

// Baselines are per path. One file's save must not silence another's conflict.
func TestBaselinesAreIndependentPerPath(t *testing.T) {
	var s Session
	s.RememberOnDisk("/a.md", "A")
	s.RememberOnDisk("/b.md", "B")
	if got, _ := s.BaselineFor("/a.md"); got != "A" {
		t.Errorf("/a.md baseline = %q, want A", got)
	}
	if _, known := s.BaselineFor("/c.md"); known {
		t.Error("an untouched path must still have no baseline")
	}
}

// A file whose content is legitimately empty must be distinguishable from one
// the app has no belief about: the first is a comparison it can make, the
// second is a reason not to interrupt the user.
func TestAnEmptyBaselineIsStillAKnownBaseline(t *testing.T) {
	var s Session
	s.RememberOnDisk("/empty.md", "")
	got, known := s.BaselineFor("/empty.md")
	if !known || got != "" {
		t.Errorf("BaselineFor = %q, %v; an empty file is known, not unknown", got, known)
	}
}
