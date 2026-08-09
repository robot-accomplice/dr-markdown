package e2e

import (
	"strings"
	"testing"
)

// Two renders that overlap must not leave the surface showing the older one.
//
// renderWysiwyg runs three passes with an await between each: rebuild the
// editor, highlight fenced code, resolve images. Nothing prevented a second
// render starting while the first was suspended, so the older pass resumed
// afterwards and stamped its state over the newer DOM.
//
// Reproduced deterministically: starting two renders back to back left a
// ONE-block document with TWO code shells, one carrying the language from the
// superseded render. It also surfaced naturally as an intermittent failure in
// TestContextualDocumentControlsManageBlocksInPlace, where the markdown was
// already correct while the DOM still showed the previous language — a user
// changing a language twice in quick succession sees a stale highlight that
// never corrects itself.
func TestOverlappingRendersLeaveTheNewestStateOnScreen(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	// Repeat: the defect is a race, so a single green run proves little.
	for i := 0; i < 5; i++ {
		var got string
		evalJS(t, ctx, "(async () => {\n"+
			"  const first = window.__app.setMarkdown('```javascript\\nconst x = 1\\n```\\n')\n"+
			"  const second = window.__app.setMarkdown('```python\\ny = 2\\n```\\n')\n"+
			"  await Promise.all([first, second])\n"+
			"  await new Promise((r) => setTimeout(r, 300))\n"+
			"  return JSON.stringify({\n"+
			"    shells: Array.from(document.querySelectorAll('#wysiwyg .code-block-shell')).map((s) => s.dataset.language)\n"+
			"  })\n"+
			"})()", &got)

		if !strings.Contains(got, `"shells":["python"]`) {
			t.Fatalf("run %d: overlapping renders left stale or duplicated state: %s\n"+
				"want exactly one shell, language python (the later render)", i, got)
		}
	}
}

// A failed render must not wedge every render after it.
//
// Serializing renders through a promise chain introduces this hazard: a
// rejection propagates, so `.then` on the poisoned chain would skip its
// callback and the surface would stop updating for the rest of the session —
// silently, with the app appearing to ignore every subsequent edit. The failure
// is recorded rather than swallowed, which is what the event trail is for.
func TestAFailedRenderDoesNotWedgeLaterRenders(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `(() => {
		globalThis.__recorded = []
		globalThis.go = { main: { App: {
			LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
			RecordClientEvent: async (e) => { globalThis.__recorded.push(e) },
			SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
		} } }
		return 'ok'
	})()`, &res)

	var outcome string
	evalJS(t, ctx, `(async () => {
		// Force one render to throw by making image resolution blow up.
		const original = Element.prototype.querySelectorAll
		let broken = true
		Element.prototype.querySelectorAll = function (...args) {
			if (broken && this.id === 'wysiwyg') { broken = false; throw new Error('forced render failure') }
			return original.apply(this, args)
		}
		try { await window.__app.setMarkdown('# One\n') } catch (e) {}
		Element.prototype.querySelectorAll = original

		// The next render must still work.
		await window.__app.setMarkdown('# Two\n')
		await new Promise((r) => setTimeout(r, 250))
		return JSON.stringify({
			text: document.getElementById('wysiwyg').textContent.slice(0, 40),
			recorded: globalThis.__recorded
		})
	})()`, &outcome)

	if !strings.Contains(outcome, "Two") {
		t.Errorf("a failed render wedged every render after it: %s", outcome)
	}
	if !strings.Contains(outcome, "render") {
		t.Errorf("the render failure was swallowed instead of recorded: %s", outcome)
	}
}
