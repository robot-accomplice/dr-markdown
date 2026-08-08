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

// Writing through a symlink must update the file the link points at, not
// replace the link. Users keep notes in vaults reached by symlink: the editor
// showed the target's content on open, accepted edits, reported a successful
// save, and left the real note untouched while the edit landed in a new
// regular file at the link's path.
func TestWriteFollowsSymlinksInsteadOfReplacingThem(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real-note.md")
	link := filepath.Join(dir, "linked-note.md")
	if err := os.WriteFile(target, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}

	if err := Write(link, []byte("edited\n"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a regular file")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "edited\n" {
		t.Fatalf("the edit did not reach the real target: %q err %v", got, err)
	}
}

// An existing file keeps its permissions. Saving used to reset every document
// to 0600, silently making a group-readable or published file owner-only.
func TestWritePreservesTheExistingFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.md")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Write(path, []byte("edited\n"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %v, want the file's existing 0644", got)
	}
}

// A file that does not exist yet takes the caller's mode.
func TestWriteUsesTheRequestedModeForANewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.md")
	if err := Write(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %v, want 0600", got)
	}
}
