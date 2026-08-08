//go:build darwin || linux

package atomicfile

import "golang.org/x/sys/unix"

// copyExtendedAttributes copies every extended attribute from src to dst.
//
// Replacing a file by rename gives up the original inode, and the extended
// attributes hang off that inode rather than off the name. On macOS they carry
// Finder tags, Spotlight comments and download provenance, so a user who tagged
// a note lost the tag the first time they saved it — silently, since nothing in
// the save path reported a partial result.
//
// Failures are returned but treated as advisory by the caller: losing a tag is
// bad, and losing the document because a filesystem does not support xattrs
// would be much worse.
func copyExtendedAttributes(src, dst string) error {
	size, err := unix.Listxattr(src, nil)
	if err != nil || size == 0 {
		return err
	}
	buf := make([]byte, size)
	n, err := unix.Listxattr(src, buf)
	if err != nil {
		return err
	}

	var firstErr error
	for _, name := range splitNames(buf[:n]) {
		valueSize, err := unix.Getxattr(src, name, nil)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		value := make([]byte, valueSize)
		if _, err := unix.Getxattr(src, name, value); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := unix.Setxattr(dst, name, value, 0); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// splitNames splits the NUL-separated attribute name list Listxattr returns.
func splitNames(buf []byte) []string {
	var names []string
	start := 0
	for i, b := range buf {
		if b == 0 {
			if i > start {
				names = append(names, string(buf[start:i]))
			}
			start = i + 1
		}
	}
	return names
}
