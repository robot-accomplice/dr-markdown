package e2e

import (
	"strings"
	"testing"
)

// A failure the user can hit must leave something durable behind.
//
// A production build has no devtools, so `console.warn` reaches nobody: a
// ReferenceError in the style-application path hid behind one for two rounds of
// debugging while the catch that swallowed it looked perfectly reasonable. The
// event trail (bridge.recordEvent -> App.RecordClientEvent -> internal/eventlog)
// exists to fix that, and until this test it had exactly ONE call site, which
// is not coverage.
//
// Each case induces a real failure and asserts an event was recorded. It does
// NOT assert the console call — that is a development convenience, and
// asserting it would let a change satisfy this test while still telling the
// maintainer nothing.
func TestFailureSurfacesRecordDurableEvents(t *testing.T) {
	// A boot-time bridge rejection is the worst case: the window is up, the
	// failure is invisible, and the user has no way to report what happened.
	t.Run("preferences load failure", func(t *testing.T) {
		ctx, cancel := newTestBrowser(t)
		defer cancel()
		url := serveFrontend(t)
		bootAppWithNativeStub(t, ctx, url, `globalThis.__events = [];
			globalThis.go = { main: { App: {
				LoadPreferences: async () => { throw new Error('decode preferences: unexpected end of JSON input') },
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SetDirty: async () => {},
				UpdateContent: async () => {}
			} } };`)

		var events string
		evalJS(t, ctx, `(globalThis.__events || []).join(',')`, &events)
		if !strings.Contains(events, "preferences.load-failed") {
			t.Errorf("a rejected preferences load recorded no event; trail = %q", events)
		}
	})

	// Refusing a link is an autonomous security decision taken against
	// untrusted document content, and it left no trace at all.
	//
	// Note where the refusal happens: at RENDER, when the anchor is built — a
	// refused link is given no href, so it can never reach the click handler at
	// all. An earlier version of this test clicked the anchor and could not have
	// passed no matter what the code did.
	t.Run("refused link scheme", func(t *testing.T) {
		ctx, cancel := newTestBrowser(t)
		defer cancel()
		url := serveFrontend(t)
		bootApp(t, ctx, url)

		var events string
		evalJS(t, ctx, `(async () => {
			globalThis.__events = []
			globalThis.go = { main: { App: {
				LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
			} } }
			await window.__app.setMarkdown('[click me](javascript:alert(1))\n')
			await window.__app.setMode('split')
			await new Promise((r) => setTimeout(r, 100))
			return (globalThis.__events || []).join(',')
		})()`, &events)
		if !strings.Contains(events, "link.refused") {
			t.Errorf("a refused link scheme recorded no event; trail = %q", events)
		}
	})

	// An audit trail a hostile document can erase is worse than none: the log is
	// trimmed and the refused link is attacker-controlled content, so a refusal
	// recorded on every render lets a document the app has ALREADY judged
	// hostile evict every other event.
	//
	// The repeat trigger is a MODE SWITCH, not typing. That was measured, not
	// assumed, and the first version of this test got it wrong: with the dedupe
	// deliberately removed, ten simulated edits still produced one record, so a
	// keystroke-driven test passed no matter what the code did. Five
	// wysiwyg/split round trips produced six. Drive it the way it actually
	// repeats, or this test is decoration.
	t.Run("a hostile document cannot flood the trail", func(t *testing.T) {
		ctx, cancel := newTestBrowser(t)
		defer cancel()
		url := serveFrontend(t)
		bootApp(t, ctx, url)

		var count int
		evalJS(t, ctx, `(async () => {
			globalThis.__events = []
			globalThis.go = { main: { App: {
				LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
			} } }
			await window.__app.setMarkdown('[a](javascript:alert(1))\n')
			await window.__app.setMode('split')
			// Re-render the same refused link the way it actually repeats.
			for (let i = 0; i < 8; i++) {
				await window.__app.setMode('wysiwyg')
				await window.__app.setMode('split')
			}
			await new Promise((r) => setTimeout(r, 100))
			return (globalThis.__events || []).filter((e) => e === 'link.refused').length
		})()`, &count)

		if count == 0 {
			t.Fatal("the refused link recorded nothing at all")
		}
		if count > 1 {
			t.Errorf("one distinct refused href recorded %d events across mode switches; "+
				"a hostile document can evict the rest of the trail by being re-rendered", count)
		}
	})

	// A rejected drop abandons the whole import rather than half-importing it.
	// That is the right behaviour and it is silent, which is the problem.
	t.Run("dropped image import rejected", func(t *testing.T) {
		ctx, cancel := newTestBrowser(t)
		defer cancel()
		url := serveFrontend(t)
		bootApp(t, ctx, url)

		var events string
		evalJS(t, ctx, `(async () => {
			globalThis.__events = []
			globalThis.go = { main: { App: {
				LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
				ImportDroppedImage: async () => { throw new Error('document must be saved first') },
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
			} } }
			await window.__app.handleDroppedFiles(['/tmp/shot.png'])
			await new Promise((r) => setTimeout(r, 100))
			return (globalThis.__events || []).join(',')
		})()`, &events)
		if !strings.Contains(events, "image.import-failed") {
			t.Errorf("a rejected image drop recorded no event; trail = %q", events)
		}
	})
}
