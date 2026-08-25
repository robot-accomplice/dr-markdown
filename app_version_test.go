package main

import (
	"os"
	"regexp"
	"testing"
)

// VERSION is the single source of build identity: Go embeds it into appVersion,
// and anything that stamps a bundle reads the same file.
//
// This project has already shipped the alternative. The plist said 1.0.0 for
// every release ever made, because the version existed in two places and
// nothing compared them. A recorded event carrying the wrong build is worse
// than one carrying none — it sends whoever is debugging to the wrong source.
func TestAppVersionComesFromTheVersionFile(t *testing.T) {
	raw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}

	// Trailing newline is expected and is trimmed into appVersion; anything else
	// around it means the file was edited by something that will do it again.
	if got := string(raw); got != appVersion+"\n" {
		t.Errorf("VERSION is %q; expected exactly %q plus one newline", got, appVersion)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(appVersion) {
		t.Errorf("appVersion %q is not a semver triple; the bundle and the event trail both carry this verbatim", appVersion)
	}
}
