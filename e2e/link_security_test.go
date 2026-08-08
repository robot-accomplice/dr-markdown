package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// A document's link must never navigate the app's own window. Nothing
// intercepted anchor clicks, so clicking an ordinary https link in the preview
// replaced the application with the remote page — no address bar, no back
// button, no way out but quitting. Links belong in the user's browser.
func TestDocumentLinksOpenExternallyAndNeverNavigateTheAppWindow(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.__opened = null;
	globalThis.go = { main: { App: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		SyncDocuments: async () => {},
		SetDirty: async () => {},
		UpdateContent: async () => {},
		OpenExternalURL: async (u) => { globalThis.__opened = u }
	} } }; 'ok'`, &res)

	var outcome string
	evalJS(t, ctx, `(async () => {
		await window.__app.setMarkdown('[docs](https://example.com/page)\n')
		await window.__app.setMode('split')
		const before = location.href
		const a = document.querySelector('#split-preview a')
		if (!a) return 'NO-ANCHOR'
		a.click()
		await new Promise((r) => setTimeout(r, 50))
		return JSON.stringify({ opened: globalThis.__opened, navigated: location.href !== before })
	})()`, &outcome)

	if !strings.Contains(outcome, `"opened":"https://example.com/page"`) {
		t.Errorf("link was not handed to the browser: %s", outcome)
	}
	if !strings.Contains(outcome, `"navigated":false`) {
		t.Errorf("clicking a document link navigated the app window itself: %s", outcome)
	}
}

// The link allowlist must refuse what the BROWSER will navigate to, not what
// the raw string looks like. The URL parser strips ASCII tab, LF and CR from
// anywhere in a URL before parsing, so a check run on the unstripped string is
// checking a different string than the one that executes: `jav<TAB>ascript:`
// has no scheme by a regex's reading and `javascript:` by the parser's.
//
// This matters more here than in a browser tab. A javascript: URL runs in the
// app's own origin, where window.go.main.App exposes SaveDocument and
// OpenRecentDocument with no path restriction — so a document that talks its
// way past this check gets arbitrary file read and write on the user's machine.
// This product exists to open ARBITRARY markdown, so every document is
// untrusted input and this is a reachable path, not a theoretical one.
func TestObfuscatedSchemesAreRefusedInRenderedLinks(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	for _, tc := range []struct {
		name  string
		href  string
		allow bool
	}{
		{"plain javascript", "javascript:alert(1)", false},
		{"tab inside scheme", "jav\tascript:alert(1)", false},
		{"uppercase with tab", "JAV\tASCRIPT:alert(1)", false},
		{"newline inside scheme", "java\nscript:alert(1)", false},
		{"carriage return inside scheme", "java\rscript:alert(1)", false},
		{"leading control character", "\x01javascript:alert(1)", false},
		{"data url", "data:text/html,<b>x</b>", false},
		{"vbscript", "vbscript:msgbox(1)", false},
		{"file url", "file:///etc/passwd", false},
		{"ordinary https", "https://example.com/page", true},
		{"mailto", "mailto:someone@example.com", true},
		{"relative document", "notes/other.md", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := json.Marshal("[click me](" + tc.href + ")\n")
			if err != nil {
				t.Fatal(err)
			}
			var got string
			evalJS(t, ctx, `(async () => {
				await window.__app.setMarkdown(`+string(doc)+`)
				await window.__app.setMode('split')
				const a = document.querySelector('#split-preview a')
				if (!a) return 'NO-ANCHOR'
				return a.dataset.blockedHref === 'true' ? 'BLOCKED' : a.protocol
			})()`, &got)

			if !tc.allow {
				// NO-ANCHOR is also a refusal: a raw newline cannot appear in a
				// markdown destination, so that input never becomes a link at all.
				if got != "BLOCKED" && got != "NO-ANCHOR" {
					t.Errorf("%q rendered as a live link (protocol %q); a document can reach the Go bindings through it", tc.href, got)
				}
				return
			}
			if got == "BLOCKED" {
				t.Errorf("%q is an ordinary link and must not be refused", tc.href)
			}
		})
	}
}
