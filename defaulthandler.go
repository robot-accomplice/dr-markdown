package main

import (
	"os"
	"strings"
)

// States of the "Set as Default Markdown Application…" menu item. The values
// cross into Objective-C (hostDefaultHandlerMenuState in host_darwin.go), so
// they are ints, not a Go enum type.
const (
	defaultHandlerOffer     = 0 // enabled, unchecked — choosing it asks the OS
	defaultHandlerIsDefault = 1 // checked, disabled — nothing to offer
	defaultHandlerDiskImage = 2 // disabled — never associate from a mounted image
)

// defaultHandlerMenuState is the whole decision, pure so it can be tested
// without Launch Services or a menu bar. The two guards and their reasons are
// in docs/decisions/2026-08-25-default-markdown-handler.md.
func defaultHandlerMenuState(isDefault bool, execPath string) int {
	if isDefault {
		return defaultHandlerIsDefault
	}
	// A bundle run straight out of the DMG must never become the default: the
	// association would point every future .md double-click at a volume that
	// will be unmounted. Gatekeeper's app translocation is the same failure
	// class with a different path shape: a quarantined app launched in place
	// runs from a randomized read-only copy under /AppTranslocation/, which
	// vanishes too. The checks are on the path, not "/Applications"
	// membership — ~/Applications and elsewhere are legitimate installs.
	if strings.HasPrefix(execPath, "/Volumes/") || strings.Contains(execPath, "/AppTranslocation/") {
		return defaultHandlerDiskImage
	}
	return defaultHandlerOffer
}

// osExecutable is a variable so tests can point the disk-image guard at a
// mounted-image path.
var osExecutable = os.Executable

// executablePath is where the running binary lives. Empty on error, which the
// decision treats as "offer" — a path we cannot read is not a disk image.
func executablePath() string {
	p, err := osExecutable()
	if err != nil {
		return ""
	}
	return p
}
