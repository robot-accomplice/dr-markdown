package main

import (
	"context"
	"testing"
)

// Clicking an ordinary https link in the preview replaced the ENTIRE app window
// with the remote page: nothing called BrowserOpenURL, no anchor carried a
// target, and no handler intercepted the click. A chrome-less desktop window
// with no address bar and no back button silently becomes someone else's page,
// and the only escape is quitting the app. External links belong in the user's
// browser, where the URL is visible and the tab is closable.
func TestExternalLinksOpenInTheBrowserNotTheAppWindow(t *testing.T) {
	native := &fakeNative{}
	app := newAppWithDependencies(appDependencies{
		native:      native,
		documents:   &fakeDocuments{},
		fonts:       fakeFonts{},
		preferences: &fakePreferences{},
	})
	app.startup(context.Background())

	if err := app.OpenExternalURL("https://example.com/page"); err != nil {
		t.Fatalf("an ordinary web link must open: %v", err)
	}
	if native.externalURL != "https://example.com/page" {
		t.Errorf("link was not handed to the browser: %q", native.externalURL)
	}
}

// Go must not trust the frontend's allowlist. The webview is where untrusted
// document content is parsed, so a bound method that hands any string to the
// OS URL opener is a second route to the same execution the frontend check
// exists to stop — and on some platforms the opener will launch a local
// handler for a scheme the browser would never navigate to.
func TestOpenExternalURLRefusesSchemesTheFrontendWouldRefuse(t *testing.T) {
	for _, raw := range []string{
		"javascript:alert(1)",
		"jav\tascript:alert(1)",
		"data:text/html,<b>x</b>",
		"file:///etc/passwd",
		"vbscript:msgbox(1)",
		"\x01javascript:alert(1)",
		"",
	} {
		native := &fakeNative{}
		app := newAppWithDependencies(appDependencies{
			native:      native,
			documents:   &fakeDocuments{},
			fonts:       fakeFonts{},
			preferences: &fakePreferences{},
		})
		app.startup(context.Background())

		if err := app.OpenExternalURL(raw); err == nil {
			t.Errorf("%q was accepted for opening", raw)
		}
		if native.externalURL != "" {
			t.Errorf("%q reached the OS URL opener: %q", raw, native.externalURL)
		}
	}
}
