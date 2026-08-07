package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesContentAndAppliesPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old and much longer content"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := Write(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("content = %q err %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v, want 0600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %d entries", len(entries))
	}
}

// The property the whole package exists for: a write that cannot complete must
// leave the previous file intact. Truncating here is what produced the corrupt
// preferences.json that prevented the app from starting (issue #17).
func TestFailedWriteLeavesExistingFileIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	original := []byte(`{"settings":{"theme":"dark"}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Deny creation of the temp file in the target directory, so the write
	// fails at the earliest step while the target still exists.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := Write(path, []byte("replacement"), 0o600); err == nil {
		t.Fatal("Write should fail when it cannot create its temp file")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original after failed write: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("failed write damaged the existing file: %q", got)
	}
}
