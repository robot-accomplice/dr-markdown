package preferences

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreLoadMissingFileReturnsEmptyPreferences(t *testing.T) {
	store := NewStore(t.TempDir(), fixedClock())

	prefs, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error for missing preferences: %v", err)
	}
	if len(prefs.Settings) != 0 {
		t.Fatalf("Settings = %v, want empty map", prefs.Settings)
	}
	if len(prefs.RawOptions) != 0 {
		t.Fatalf("RawOptions = %v, want empty map", prefs.RawOptions)
	}
	if len(prefs.Recents) != 0 {
		t.Fatalf("Recents = %v, want empty list", prefs.Recents)
	}
}

func TestStoreSaveAndLoadRoundTripsPreferences(t *testing.T) {
	store := NewStore(t.TempDir(), fixedClock())
	input := Preferences{
		Settings: map[string]any{
			"theme":            "dark",
			"documentFont":     "Georgia",
			"documentFontSize": float64(17),
			"codeLigatures":    false,
		},
		RawOptions: map[string]any{
			"lineNumbers": false,
		},
		Recents: []RecentDocument{{
			Path:         "/tmp/design.md",
			Title:        "design.md",
			LastOpenedAt: "2026-08-07T13:00:00Z",
		}},
	}

	if err := store.Save(input); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Settings["theme"] != "dark" || loaded.Settings["documentFont"] != "Georgia" {
		t.Fatalf("Settings did not round-trip: %#v", loaded.Settings)
	}
	if loaded.RawOptions["lineNumbers"] != false {
		t.Fatalf("RawOptions did not round-trip: %#v", loaded.RawOptions)
	}
	if len(loaded.Recents) != 1 || loaded.Recents[0].Path != "/tmp/design.md" {
		t.Fatalf("Recents did not round-trip: %#v", loaded.Recents)
	}
}

func TestStoreRecordRecentDeduplicatesAndKeepsNewestFirst(t *testing.T) {
	store := NewStore(t.TempDir(), fixedClock())

	if _, err := store.RecordRecent("/tmp/older.md"); err != nil {
		t.Fatalf("RecordRecent older: %v", err)
	}
	if _, err := store.RecordRecent("/tmp/newer.md"); err != nil {
		t.Fatalf("RecordRecent newer: %v", err)
	}
	recents, err := store.RecordRecent("/tmp/older.md")
	if err != nil {
		t.Fatalf("RecordRecent duplicate: %v", err)
	}

	if len(recents) != 2 {
		t.Fatalf("recents length = %d, want 2", len(recents))
	}
	if recents[0].Path != "/tmp/older.md" || recents[0].Title != "older.md" {
		t.Fatalf("duplicate recent should move to front with title from basename: %#v", recents)
	}
	if recents[1].Path != "/tmp/newer.md" {
		t.Fatalf("newer item should remain after moved duplicate: %#v", recents)
	}
	if recents[0].LastOpenedAt != "2026-08-07T13:00:00Z" {
		t.Fatalf("LastOpenedAt = %q, want fixed clock timestamp", recents[0].LastOpenedAt)
	}
}

func TestStoreLoadRejectsCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt fixture: %v", err)
	}
	store := NewStore(dir, fixedClock())

	if _, err := store.Load(); err == nil {
		t.Fatal("Load should reject corrupt preferences JSON")
	}
}

func TestStoreSaveNormalizesNilCollections(t *testing.T) {
	store := NewStore(t.TempDir(), fixedClock())

	if err := store.Save(Preferences{}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Settings == nil || loaded.RawOptions == nil || loaded.Recents == nil {
		t.Fatalf("nil collections should normalize to empty collections: %#v", loaded)
	}
}

func TestStoreSaveReturnsDirectoryCreationError(t *testing.T) {
	dir := t.TempDir()
	fileAsDir := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	store := NewStore(fileAsDir, fixedClock())

	if err := store.Save(Preferences{}); err == nil {
		t.Fatal("Save should fail when the preferences directory cannot be created")
	}
}

func TestStoreRecordRecentPropagatesLoadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt fixture: %v", err)
	}
	store := NewStore(dir, fixedClock())

	if _, err := store.RecordRecent("/tmp/doc.md"); err == nil {
		t.Fatal("RecordRecent should return corrupt preference load errors")
	}
}

func TestStoreRecordRecentCapsList(t *testing.T) {
	store := NewStore(t.TempDir(), fixedClock())

	for i := 0; i < 25; i++ {
		if _, err := store.RecordRecent(filepath.Join("/tmp", string(rune('a'+i))+".md")); err != nil {
			t.Fatalf("RecordRecent %d: %v", i, err)
		}
	}
	prefs, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(prefs.Recents) != 20 {
		t.Fatalf("recents length = %d, want 20", len(prefs.Recents))
	}
}

func TestNewStoreUsesRealtimeClockWhenNil(t *testing.T) {
	store := NewStore(t.TempDir(), nil)

	recents, err := store.RecordRecent("/tmp/doc.md")
	if err != nil {
		t.Fatalf("RecordRecent returned error: %v", err)
	}
	if len(recents) != 1 || recents[0].LastOpenedAt == "" {
		t.Fatalf("nil clock should still stamp recents: %#v", recents)
	}
}

func fixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)
	}
}
