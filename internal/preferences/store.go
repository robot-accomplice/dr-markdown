// Package preferences persists user preferences and recent documents.
package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dr-markdown/internal/atomicfile"
)

const maxRecents = 20

// Preferences is the native persistence envelope shared with the frontend.
type Preferences struct {
	Settings   map[string]any   `json:"settings"`
	RawOptions map[string]any   `json:"rawOptions"`
	Recents    []RecentDocument `json:"recents"`
}

// RecentDocument is a local markdown file shown on the start screen.
type RecentDocument struct {
	Path         string `json:"path"`
	Title        string `json:"title"`
	LastOpenedAt string `json:"lastOpenedAt"`
}

// Store reads and writes preferences.json in a caller-selected config dir.
type Store struct {
	path string
	now  func() time.Time
}

// NewStore creates a preferences store rooted at configDir.
func NewStore(configDir string, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{path: filepath.Join(configDir, "preferences.json"), now: now}
}

// DefaultStore creates the production preferences store.
func DefaultStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate user config dir: %w", err)
	}
	// The directory keeps the ORIGINAL name. Renaming it to match the
	// application would silently abandon the user's settings and recents on
	// upgrade, which is a worse outcome than a directory whose name is a
	// release behind. The user never sees it.
	return NewStore(filepath.Join(dir, "Dr. Markdown"), time.Now), nil
}

// Load returns persisted preferences, or empty preferences when none exist.
//
// A malformed file is recovered from rather than reported: preferences are an
// enhancement, and failing here prevented the application from starting at all
// (issue #17). The bad file is kept alongside so the failure stays diagnosable
// instead of being silently discarded.
func (s *Store) Load() (Preferences, error) {
	if _, err := os.Stat(s.path); err != nil {
		if os.IsNotExist(err) {
			return emptyPreferences(), nil
		}
		return Preferences{}, fmt.Errorf("stat preferences: %w", err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Preferences{}, fmt.Errorf("read preferences: %w", err)
	}
	var prefs Preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		s.quarantineCorruptFile(data)
		return emptyPreferences(), nil
	}
	normalize(&prefs)
	return prefs, nil
}

// quarantineCorruptFile preserves an unparseable preferences file next to the
// original. Best-effort: recovery must not depend on the backup succeeding.
func (s *Store) quarantineCorruptFile(data []byte) {
	backup := fmt.Sprintf("%s.corrupt-%s", s.path, s.now().UTC().Format("20060102T150405Z"))
	_ = os.WriteFile(backup, data, 0o600)
}

// Save writes preferences to disk.
func (s *Store) Save(prefs Preferences) error {
	normalize(&prefs)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create preferences dir: %w", err)
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}
	data = append(data, '\n')
	// Atomic: a crash mid-write must not produce the truncated file that Load
	// now has to recover from.
	if err := atomicfile.Write(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write preferences: %w", err)
	}
	return nil
}

// RecordRecent moves path to the top of the recent document list.
func (s *Store) RecordRecent(path string) ([]RecentDocument, error) {
	prefs, err := s.Load()
	if err != nil {
		return nil, err
	}
	next := RecentDocument{
		Path:         path,
		Title:        filepath.Base(path),
		LastOpenedAt: s.now().UTC().Format(time.RFC3339),
	}
	recents := []RecentDocument{next}
	for _, recent := range prefs.Recents {
		if recent.Path == path {
			continue
		}
		recents = append(recents, recent)
		if len(recents) == maxRecents {
			break
		}
	}
	prefs.Recents = recents
	if err := s.Save(prefs); err != nil {
		return nil, err
	}
	return recents, nil
}

func emptyPreferences() Preferences {
	return Preferences{
		Settings:   map[string]any{},
		RawOptions: map[string]any{},
		Recents:    []RecentDocument{},
	}
}

func normalize(prefs *Preferences) {
	if prefs.Settings == nil {
		prefs.Settings = map[string]any{}
	}
	if prefs.RawOptions == nil {
		prefs.RawOptions = map[string]any{}
	}
	if prefs.Recents == nil {
		prefs.Recents = []RecentDocument{}
	}
}
