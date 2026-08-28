package e2e

import (
	"encoding/json"
	"strconv"
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
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		SyncDocuments: async () => {},
		SetDirty: async () => {},
		UpdateContent: async () => {},
		OpenExternalURL: async (u) => { globalThis.__opened = u }
	} } ; 'ok'`, &res)

	var outcome string
	evalJS(t, ctx, `(async () => {
		await window.__app.setMarkdown('[docs](https://example.com/page)\n')
		await window.__app.setMode('split')
		const before = location.href
		// Split shows the real editor, so this is the anchor the user actually
		// clicks. The handler under test is bound to the whole document
		// precisely so it covers any anchor that reaches it, not only ones the
		// app's own renderer built.
		const a = document.querySelector('#wysiwyg a')
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
// app's own origin, where globalThis.drmd.native exposes SaveDocument and
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
				await window.__app.printDocument('print')
				const a = document.querySelector('#print-root a')
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

// A mermaid diagram link is an SVG anchor carrying its destination in the XLINK
// namespace and NO plain href. The guard used to select `a[href]`, and an
// unprefixed CSS attribute selector matches only null-namespace attributes — so
// a diagram in an opened document slipped past it entirely and WebKit performed
// the navigation. The new page then inherited the bridge, which is installed for
// the main frame with no origin restriction and exposes SaveDocument and
// OpenRecentDocument.
//
// The test above could not have caught this: it clicks an HTML anchor, which is
// the one shape the old selector did match. This one asserts the property that
// actually matters — the guard reads the destination a click will FOLLOW, not
// the spelling this application happens to write. (#145)
//
// What this CANNOT cover: the second layer. The host now refuses any main-frame
// navigation off the app's own scheme in its WKNavigationDelegate, which is
// native and unreachable from chromedp — so `navigated` below stays false here
// whether or not that delegate exists. The property this test does pin is that
// the guard SEES the anchor at all, which is exactly what failed: with the old
// `a[href]` selector the ordinary link is never handed to the browser, because
// the handler returned before it could be.
func TestNamespacedSvgLinksAreRefusedLikeAnyOther(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.__opened = null; globalThis.__events = [];
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {},
		RecordClientEvent: (event) => { globalThis.__events.push(event) },
		OpenExternalURL: async (u) => { globalThis.__opened = u }
	} } ; 'ok'`, &res)

	for _, tc := range []struct {
		name    string
		href    string
		allowed bool
	}{
		{"https diagram link is handed to the browser", "https://example.com/page", true},
		{"javascript diagram link is refused", "javascript:alert(1)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var outcome string
			evalJS(t, ctx, `(async () => {
				globalThis.__opened = null
				const NS = 'http://www.w3.org/1999/xlink'
				const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
				const a = document.createElementNS('http://www.w3.org/2000/svg', 'a')
				// Exactly what mermaid emits: setAttributeNS, and no plain href.
				a.setAttributeNS(NS, 'xlink:href', `+strconv.Quote(tc.href)+`)
				const label = document.createElementNS('http://www.w3.org/2000/svg', 'text')
				label.textContent = 'node'
				a.appendChild(label); svg.appendChild(a)
				// document.body, not the editor: the guard is bound document-wide,
				// and depending on app DOM would make this test fail for reasons
				// that have nothing to do with the guard.
				document.body.appendChild(svg)

				const before = location.href
				let defaultPrevented = false
				label.addEventListener('click', (e) => { defaultPrevented = e.defaultPrevented }, { once: true })
				label.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
				await new Promise((r) => setTimeout(r, 50))
				svg.remove()
				return JSON.stringify({
					opened: globalThis.__opened,
					navigated: location.href !== before,
				})
			})()`, &outcome)

			if strings.Contains(outcome, `"navigated":true`) {
				t.Fatalf("a namespaced diagram link navigated the app window: %s", outcome)
			}
			if tc.allowed {
				if !strings.Contains(outcome, `"opened":"`+tc.href+`"`) {
					t.Errorf("an ordinary diagram link should be handed to the browser: %s", outcome)
				}
				return
			}
			if strings.Contains(outcome, `"opened":"`+tc.href+`"`) {
				t.Errorf("a refused scheme reached the browser through a diagram link: %s", outcome)
			}
		})
	}

	// The refusal must be recorded, like every other one.
	var recorded bool
	evalJS(t, ctx, `(globalThis.__events || []).includes('link.refused')`, &recorded)
	if !recorded {
		t.Error("refusing a namespaced diagram link recorded nothing")
	}
}
