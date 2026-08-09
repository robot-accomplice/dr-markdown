//go:build !darwin && !linux

package atomicfile

// copyExtendedAttributes is a no-op on platforms without POSIX extended
// attributes. Windows alternate data streams are a different mechanism and are
// not carried by this path; that is recorded rather than silently implied,
// since this ships as one binary for three platforms.
func copyExtendedAttributes(src, dst string) error { return nil }
