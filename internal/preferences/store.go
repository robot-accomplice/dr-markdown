// Package preferences persists user preferences and recent documents.
package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	return NewStore(filepath.Join(dir, "Dr. Markdown"), time.Now), nil
}

// Load returns persisted preferences, or empty preferences when none exist.
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
		return Preferences{}, fmt.Errorf("decode preferences: %w", err)
	}
	normalize(&prefs)
	return prefs, nil
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
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
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
