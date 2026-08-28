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
			globalThis.drmd = { native: {
				LoadPreferences: async () => { throw new Error('decode preferences: unexpected end of JSON input') },
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SetDirty: async () => {},
				UpdateContent: async () => {}
			} } ;`)

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
			globalThis.drmd = { native: {
				LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
			} } ; window.print = () => {} 
			await window.__app.setMarkdown('[click me](javascript:alert(1))\n')
			await window.__app.printDocument('print')
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
			globalThis.drmd = { native: {
				LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
			} } ; window.print = () => {} 
			await window.__app.setMarkdown('[a](javascript:alert(1))\n')
			// Re-render the same refused link the way it actually repeats. The
			// driver is a repeated PRINT rather than a mode switch: the refusal
			// is made by the renderer, and since split became the real editor
			// the renderer runs for print and nothing else.
			for (let i = 0; i < 9; i++) {
				await window.__app.printDocument('print')
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

	// An uncaught error and an unhandled rejection reach the trail.
	//
	// The host already listened for both, but into an array only the -gates
	// harness reads — so in a shipped build anything nobody anticipated was
	// dropped silently while the handlers existed and read as coverage. That
	// matters most for what is newest: the async paths added for find and
	// replace await a remount with no catch of their own.
	t.Run("uncaught errors and rejections are recorded", func(t *testing.T) {
		ctx, cancel := newTestBrowser(t)
		defer cancel()
		url := serveFrontend(t)
		bootApp(t, ctx, url)

		var events string
		evalJS(t, ctx, `(async () => {
			globalThis.__events = []
			globalThis.drmd = { native: {
				LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
			} }
			// Both platform signals, raised the way the platform raises them.
			window.dispatchEvent(new ErrorEvent('error', {
				message: 'boom', filename: 'x.js', lineno: 1
			}))
			window.dispatchEvent(new PromiseRejectionEvent('unhandledrejection', {
				promise: Promise.reject(new Error('nope')).catch(() => {}),
				reason: new Error('nope')
			}))
			await new Promise((r) => setTimeout(r, 100))
			return (globalThis.__events || []).join(',')
		})()`, &events)

		if !strings.Contains(events, "error.uncaught") {
			t.Errorf("an uncaught error recorded nothing; trail = %q", events)
		}
		if !strings.Contains(events, "error.unhandled-rejection") {
			t.Errorf("an unhandled rejection recorded nothing; trail = %q", events)
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
			globalThis.drmd = { native: {
				LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
				ImportDroppedImage: async () => { throw new Error('document must be saved first') },
				RecordClientEvent: (event) => { globalThis.__events.push(event) },
				SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
			} } ; window.print = () => {} 
			await window.__app.handleDroppedFiles(['/tmp/shot.png'])
			await new Promise((r) => setTimeout(r, 100))
			return (globalThis.__events || []).join(',')
		})()`, &events)
		if !strings.Contains(events, "image.import-failed") {
			t.Errorf("a rejected image drop recorded no event; trail = %q", events)
		}
	})
}

// macOS routes a double-clicked .md file to the app, and for a long time
// nothing consumed it: the app opened an empty document and the file the user
// clicked was silently gone (#53). At launch the file arrives BEFORE the
// webview exists, so the frontend has to ask for it rather than be told.
func TestFrontendOpensAFileHandedToItAtLaunch(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)

	bootAppWithNativeStub(t, ctx, url, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
		FrontendReady: async () => ['/tmp/handed-over.md'],
		OpenRecentDocument: async (p) => ({ path: p, content: '# Handed over\n\nfrom Finder\n' }),
		SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
	} } ;`)

	var got string
	evalJS(t, ctx, `(async () => {
		for (let i = 0; i < 100; i++) {
			const doc = window.__app.state.docs.find((d) => d.id === window.__app.state.activeDocId)
			if (doc && doc.path) return JSON.stringify({ path: doc.path, markdown: window.__app.getEditorMarkdown() })
			await new Promise((r) => setTimeout(r, 20))
		}
		return 'NEVER OPENED'
	})()`, &got)

	if !strings.Contains(got, "/tmp/handed-over.md") {
		t.Errorf("a file handed over at launch was not opened: %s", got)
	}
	if !strings.Contains(got, "Handed over") {
		t.Errorf("the document opened but its content is wrong: %s", got)
	}
}
