// Package document handles reading and atomically writing markdown files.
package document

import (
	"dr-markdown/internal/atomicfile"
	"fmt"
	"os"
)

// Read loads the file at path and returns its content as a string.
func Read(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

// WriteAtomic writes content to path atomically: it writes to a temp file in
// the same directory, fsyncs, closes, then renames over the target. A crash
// mid-save can never leave a truncated document behind.
//
// The mechanism lives in internal/atomicfile so documents and preferences
// share one implementation rather than two copies that can drift apart.
func WriteAtomic(path string, content string) error {
	return atomicfile.Write(path, []byte(content), 0o600)
}
