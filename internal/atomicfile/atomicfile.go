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
//
// KNOWN AND DELIBERATE: replacing by rename breaks hard links to the document.
// The replacement is a new inode, so another name pointing at the old one keeps
// the old bytes. This is NOT fixed, because the alternative — truncating and
// rewriting in place — is exactly the non-atomic write this package exists to
// prevent, and a crash mid-write would leave a truncated document. Losing the
// link between two names is recoverable; losing the contents is not. Extended
// attributes are carried across explicitly below, since those could be
// preserved without giving up atomicity. Recorded in the README's known
// limitations.
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
	existed := false
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
		existed = true
		// Refuse a read-only target. Rename needs write permission on the
		// DIRECTORY, not on the file, so a file the user deliberately made
		// read-only was replaced anyway — defeating the exact mechanism people
		// use to protect a reference document from being overwritten.
		if perm&0o200 == 0 {
			return fmt.Errorf("write %s: file is read-only", path)
		}
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
	// Carry the original's extended attributes onto the replacement. Best
	// effort: losing a Finder tag is bad, losing the document because the
	// filesystem has no xattr support would be far worse, so a failure here
	// does not fail the save.
	if existed {
		_ = copyExtendedAttributes(path, tmpName)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file over %s: %w", path, err)
	}
	return nil
}
