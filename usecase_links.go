package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// linkUseCase owns opening external URLs in the user's browser.
type linkUseCase struct {
	native nativePort
}

// safeExternalSchemes is the Go-side allowlist for opening a URL in the user's
// browser. It deliberately duplicates the frontend check rather than trusting
// it: the webview is where untrusted document content is parsed, so a bound
// method that hands any string to the OS URL opener is a second route to the
// execution the frontend check exists to prevent — and the OS opener will
// happily launch a registered local handler for a scheme a browser would never
// navigate to.
var safeExternalSchemes = map[string]bool{"http": true, "https": true, "mailto": true}

// openExternal opens a web link in the user's browser.
//
// Without it, clicking a link in the preview navigated the app's own window to
// the remote page — a chrome-less window with no address bar and no back
// button, from which the only escape is quitting.
func (uc *linkUseCase) openExternal(ctx context.Context, raw string) error {
	// Strip exactly what a URL parser strips, so this check cannot be fooled by
	// a string that reads as harmless here and as `javascript:` to the opener.
	cleaned := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, raw)
	cleaned = strings.TrimFunc(cleaned, func(r rune) bool { return r <= ' ' })

	parsed, err := url.Parse(cleaned)
	if err != nil {
		return fmt.Errorf("open link: %q is not a URL", raw)
	}
	if !safeExternalSchemes[strings.ToLower(parsed.Scheme)] {
		return fmt.Errorf("open link: refusing scheme %q", parsed.Scheme)
	}
	return uc.native.OpenExternalURL(ctx, cleaned)
}
