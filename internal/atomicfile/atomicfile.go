// Package atomicfile writes a file so a crash or a failed write can never
// leave a truncated one behind.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write writes data to path atomically: it writes to a temp file in the same
// directory, fsyncs, closes, then renames over the target. A crash mid-write
// leaves either the previous file or the new one, never a partial file.
//
// Both callers that persist state on this machine use it — documents and
// preferences — so the guarantee holds for every file the app owns rather than
// only the one whose corruption was noticed first.
func Write(path string, data []byte, perm os.FileMode) error {
	// Write THROUGH a symlink, not over it. Replacing the link would leave the
	// user's edit in a new regular file at the link's path while the real note
	// kept its old bytes — the save reported success and the document the user
	// believed they were editing never changed. Users keep notes in vaults
	// reached by symlink, so this is an ordinary layout, not an exotic one.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	// Keep the file's existing permissions. A fresh temp file would otherwise
	// impose the caller's mode on every save, silently turning a
	// group-readable or published document owner-only.
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file over %s: %w", path, err)
	}
	return nil
}
