package e2e

import (
	"testing"
)

// Every stylesheet the app pulls in must actually load, including the ones
// reached through @import.
//
// Crepe's vendored theme @imports four stylesheets from OTHER npm packages by
// bare specifier — ProseMirror's base editor styles, the gap cursor, the
// virtual cursor and the table styles. A browser resolves a bare specifier in
// CSS relative to the importing sheet's URL, so each became a request for a
// path that did not exist and 404ed, and the editor ran without any of them.
//
// It shipped because a failed @import is SILENT: the importing sheet still
// applies, nothing throws, and no console error appears. It was found by the
// host harness logging ASSET MISS while driving the real app — a log nothing
// read.
//
// The check is `CSSImportRule.styleSheet`, which is null when the import failed
// and non-null when it resolved. That is the only observable that distinguishes
// the two, and it catches the whole class rather than the four known paths.
func TestEveryStylesheetImportResolves(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	// The editor's theme is appended at runtime by loadTheme(), so wait for it
	// rather than sampling whatever is in <head> at boot.
	if !waitForJS(t, ctx, `Array.from(document.styleSheets).some((s) => (s.href || '').includes('vendor/theme/'))`) {
		t.Fatal("the vendored editor theme never loaded at all")
	}

	var failed string
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 600))
		const bad = []
		const walk = (sheet, from) => {
			let rules
			try {
				rules = sheet.cssRules
			} catch (e) {
				// A cross-origin sheet cannot be inspected. There are none here —
				// the CSP is script-src 'self' and every asset is local — so this
				// is worth reporting rather than skipping silently.
				bad.push('unreadable: ' + (sheet.href || from))
				return
			}
			for (const rule of rules) {
				if (rule.type !== CSSRule.IMPORT_RULE) continue
				if (!rule.styleSheet) {
					bad.push((sheet.href || from) + ' -> ' + rule.href + ' FAILED TO LOAD')
					continue
				}
				walk(rule.styleSheet, rule.href)
			}
		}
		for (const sheet of document.styleSheets) walk(sheet, '(inline)')
		return JSON.stringify(bad)
	})()`, &failed)

	if failed != "[]" {
		t.Errorf("stylesheets the app asks for do not resolve, and a failed @import is silent:\n  %s",
			failed)
	}
}
