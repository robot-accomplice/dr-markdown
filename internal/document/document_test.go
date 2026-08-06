package document

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAtomicCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")

	if err := WriteAtomic(path, "# Hello\n"); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "# Hello\n" {
		t.Errorf("content = %q, want %q", got, "# Hello\n")
	}
}

func TestWriteAtomicOverwritesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")

	if err := WriteAtomic(path, "old"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteAtomic(path, "new"); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteAtomicBadDir(t *testing.T) {
	err := WriteAtomic(filepath.Join(t.TempDir(), "missing", "doc.md"), "x")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
	if !strings.Contains(err.Error(), "temp") {
		t.Errorf("error should mention temp file, got: %v", err)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != "content\n" {
		t.Errorf("Read = %q, want %q", got, "content\n")
	}

	if _, err := Read(filepath.Join(dir, "nope.md")); err == nil {
		t.Error("expected error for missing file")
	}
}
