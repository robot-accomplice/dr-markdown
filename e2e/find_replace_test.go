package e2e

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

const findFixture = "# Widget notes\n\nThe widget is small.\n\nAnother widget here.\n"

// The find bar opens on the shortcut and reports a count.
//
// The count is the part that has to be right in every mode: it comes from the
// markdown source, so Formatted, Raw and Split must agree about it. A search
// that ran over whichever surface was visible would not, and in Split — which
// shows two surfaces at once — it would disagree with itself.
func TestFindReportsTheSameMatchCountInEveryMode(t *testing.T) {
	for _, mode := range []string{"wysiwyg", "raw", "split"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := newTestBrowser(t)
			defer cancel()
			url := serveFrontend(t)
			bootApp(t, ctx, url)

			var res string
			evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(findFixture)+").then(() => 'ok')", &res)
			evalJS(t, ctx, "window.__app.setMode("+strconv.Quote(mode)+").then(() => 'ok')", &res)

			openFindBar(t, ctx)
			sendKeysTo(t, ctx, "#find-input", "widget")

			var count string
			if !waitForJS(t, ctx, `document.getElementById('find-count').textContent.includes(' of ')`) {
				evalJS(t, ctx, `document.getElementById('find-count').textContent`, &count)
				t.Fatalf("find never reported a position in %s mode; count = %q", mode, count)
			}
			evalJS(t, ctx, `document.getElementById('find-count').textContent`, &count)

			// Three, not two: the heading's "Widget" counts, because the search
			// is case-insensitive by default and runs over the source.
			if !strings.HasSuffix(count, "of 3") {
				t.Errorf("%s mode reported %q, want 3 matches — every mode searches the same source", mode, count)
			}
		})
	}
}

// Next and previous walk the matches and wrap.
func TestFindNavigatesAndWraps(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(findFixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)
	openFindBar(t, ctx)
	sendKeysTo(t, ctx, "#find-input", "widget")
	waitForJS(t, ctx, `document.getElementById('find-count').textContent.includes(' of ')`)

	var first string
	evalJS(t, ctx, `document.getElementById('find-count').textContent`, &first)

	step := func() string {
		var got string
		evalJS(t, ctx, `(() => { document.getElementById('find-next').click(); return document.getElementById('find-count').textContent })()`, &got)
		return got
	}
	second := step()
	if second == first {
		t.Fatalf("next match did not advance: still %q", second)
	}
	// Walk all the way round; the position must come back to where it started
	// rather than sticking at the last match.
	third := step()
	wrapped := step()
	if wrapped != first {
		t.Errorf("navigation did not wrap: %q -> %q -> %q -> %q", first, second, third, wrapped)
	}
}

// A match must actually be SELECTED in the source substrates, not merely
// counted. A find bar that reports "1 of 3" without showing you where is a
// counter, not a find.
func TestFindSelectsTheMatchInTheSourceSubstrates(t *testing.T) {
	for _, tc := range []struct {
		mode     string
		selector string
	}{
		{"raw", "#raw textarea"},
		{"split", "#split-source"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ctx, cancel := newTestBrowser(t)
			defer cancel()
			url := serveFrontend(t)
			bootApp(t, ctx, url)

			var res string
			evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(findFixture)+").then(() => 'ok')", &res)
			evalJS(t, ctx, "window.__app.setMode("+strconv.Quote(tc.mode)+").then(() => 'ok')", &res)
			openFindBar(t, ctx)
			sendKeysTo(t, ctx, "#find-input", "small")
			waitForJS(t, ctx, `document.getElementById('find-count').textContent.includes(' of ')`)

			var selected string
			evalJS(t, ctx, `(() => {
				const el = document.querySelector(`+strconv.Quote(tc.selector)+`)
				return el.value.slice(el.selectionStart, el.selectionEnd)
			})()`, &selected)
			if selected != "small" {
				t.Errorf("%s: selection is %q, want the match itself", tc.mode, selected)
			}
		})
	}
}

// Replace rewrites the source and the document follows. It goes through the
// source rather than the editor deliberately: an edit driven through the editor
// re-serializes the whole document, so a replace-all could rewrite lines the
// user never touched.
func TestReplaceRewritesTheDocument(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(findFixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)
	openFindReplaceBar(t, ctx)
	sendKeysTo(t, ctx, "#find-input", "widget")
	waitForJS(t, ctx, `document.getElementById('find-count').textContent.includes(' of ')`)
	sendKeysTo(t, ctx, "#replace-input", "component")

	// One replacement first: it must change exactly one occurrence.
	evalJS(t, ctx, `(() => { document.getElementById('replace-one').click(); return 'ok' })()`, &res)
	if !waitForJS(t, ctx, `window.__app.getMarkdown().includes('component')`) {
		t.Fatal("replace did not change the document")
	}
	var afterOne string
	evalJS(t, ctx, `window.__app.getMarkdown()`, &afterOne)
	if strings.Count(afterOne, "component") != 1 {
		t.Errorf("replace changed %d occurrences, want exactly 1: %q",
			strings.Count(afterOne, "component"), afterOne)
	}

	// Then all the rest.
	evalJS(t, ctx, `(() => { document.getElementById('replace-all').click(); return 'ok' })()`, &res)
	if !waitForJS(t, ctx, `!/widget/i.test(window.__app.getMarkdown())`) {
		var got string
		evalJS(t, ctx, `window.__app.getMarkdown()`, &got)
		t.Fatalf("replace all left matches behind: %q", got)
	}
	var afterAll string
	evalJS(t, ctx, `window.__app.getMarkdown()`, &afterAll)
	if strings.Count(afterAll, "component") != 3 {
		t.Errorf("replace all produced %d replacements, want 3: %q",
			strings.Count(afterAll, "component"), afterAll)
	}
	// The rest of the document must be untouched — this is the re-serialization
	// hazard the source-based design exists to avoid. Structure included: the
	// heading is still a heading and the blank lines are still where they were.
	//
	// The replacement is literal, so a case-insensitive match on "Widget"
	// becomes exactly the text typed. That is what every editor without an
	// explicit preserve-case option does, and guessing the caller's intended
	// capitalisation is worse than doing what they typed.
	if afterAll != "# component notes\n\nThe component is small.\n\nAnother component here.\n" {
		t.Errorf("replace all disturbed the document beyond the matches: %q", afterAll)
	}
}

// An unusable regular expression is reported. Answering "no results" is
// indistinguishable from a document that genuinely lacks the text, so a typo
// would read as an answer.
func TestFindReportsAnInvalidPatternRatherThanNoResults(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(findFixture)+").then(() => 'ok')", &res)
	openFindBar(t, ctx)
	evalJS(t, ctx, `(() => { document.getElementById('find-regex').click(); return 'ok' })()`, &res)
	sendKeysTo(t, ctx, "#find-input", "(unclosed")

	if !waitForJS(t, ctx, `document.getElementById('find-count').textContent === 'Invalid pattern'`) {
		var got string
		evalJS(t, ctx, `document.getElementById('find-count').textContent`, &got)
		t.Fatalf("an invalid pattern reported %q, not that it was invalid", got)
	}
	var invalid bool
	evalJS(t, ctx, `document.getElementById('find-input').getAttribute('aria-invalid') === 'true'`, &invalid)
	if !invalid {
		t.Error("the field itself should be marked invalid, not just the count")
	}
}

// Escape closes the bar, and the shortcut opens it. Without the close the bar
// is a mode the user cannot leave from the keyboard.
func TestFindBarOpensOnShortcutAndClosesOnEscape(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(findFixture)+").then(() => 'ok')", &res)

	var hiddenAtRest bool
	evalJS(t, ctx, `document.getElementById('find-bar').hidden`, &hiddenAtRest)
	if !hiddenAtRest {
		t.Error("the find bar should not be showing until it is asked for")
	}

	openFindBar(t, ctx)
	var open, focused bool
	evalJS(t, ctx, `!document.getElementById('find-bar').hidden`, &open)
	evalJS(t, ctx, `document.activeElement === document.getElementById('find-input')`, &focused)
	if !open {
		t.Fatal("the find shortcut did not open the find bar")
	}
	if !focused {
		t.Error("opening find should put the caret in the find field")
	}

	// Replace is hidden for a plain find, so the common case is one row.
	var replaceShown bool
	evalJS(t, ctx, `getComputedStyle(document.querySelector('[data-find-replace-row]')).display !== 'none'`, &replaceShown)
	if replaceShown {
		t.Error("the replace row should be hidden for a plain find")
	}

	pressEscape(t, ctx)
	var closed bool
	if !waitForJS(t, ctx, `document.getElementById('find-bar').hidden`) {
		evalJS(t, ctx, `document.getElementById('find-bar').hidden`, &closed)
		t.Error("Escape did not close the find bar")
	}
}

// openFindBar sends the find shortcut the same way the other shortcut tests do.
func openFindBar(t *testing.T, ctx context.Context) {
	t.Helper()
	sendFindShortcut(t, ctx, false)
}

// openFindReplaceBar sends the find-and-replace shortcut. It is a separate
// entry point because the replace row is HIDDEN for a plain find, so a test
// that opens plain find cannot type into it — which is the behaviour, not a
// limitation of the test.
func openFindReplaceBar(t *testing.T, ctx context.Context) {
	t.Helper()
	sendFindShortcut(t, ctx, true)
}

func sendFindShortcut(t *testing.T, ctx context.Context, withReplace bool) {
	t.Helper()
	var res string
	evalJS(t, ctx, `document.dispatchEvent(new KeyboardEvent('keydown', {
		key: 'f',
		metaKey: true,
		altKey: `+strconv.FormatBool(withReplace)+`,
		bubbles: true,
		cancelable: true
	})); 'ok'`, &res)
}

func pressEscape(t *testing.T, ctx context.Context) {
	t.Helper()
	var res string
	evalJS(t, ctx, `document.dispatchEvent(new KeyboardEvent('keydown', {
		key: 'Escape',
		bubbles: true,
		cancelable: true
	})); 'ok'`, &res)
}

// Replace All is the only single click in this product that rewrites the whole
// document, and applying it remounts the editor — which discards ProseMirror's
// undo history. The Edit menu advertises Undo and the OS delivers Cmd-Z, so
// before this the user pressed the key the system offered and the document did
// not come back. (#147)
func TestReplaceAllCanBeReverted(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(findFixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)
	openFindReplaceBar(t, ctx)
	sendKeysTo(t, ctx, "#find-input", "widget")
	waitForJS(t, ctx, `document.getElementById('find-count').textContent.includes(' of ')`)
	sendKeysTo(t, ctx, "#replace-input", "component")

	evalJS(t, ctx, `(() => { document.getElementById('replace-all').click(); return 'ok' })()`, &res)
	if !waitForJS(t, ctx, `!/widget/i.test(window.__app.getMarkdown())`) {
		t.Fatal("replace all did not run")
	}

	// Cmd-Z, the key the menu promises.
	evalJS(t, ctx, `document.dispatchEvent(new KeyboardEvent('keydown', {
		key: 'z', metaKey: true, bubbles: true, cancelable: true
	})); 'ok'`, &res)

	if !waitForJS(t, ctx, `window.__app.getMarkdown().includes('widget')`) {
		var got string
		evalJS(t, ctx, `window.__app.getMarkdown()`, &got)
		t.Fatalf("Cmd-Z did not revert Replace All; document = %q", got)
	}
	var restored string
	evalJS(t, ctx, `window.__app.getMarkdown()`, &restored)
	if restored != findFixture {
		t.Errorf("revert did not restore the document exactly:\n got  %q\n want %q", restored, findFixture)
	}
}

// The snapshot is one step deep and must not outlive an unrelated edit —
// restoring it then would discard whatever the user typed since, which is the
// same data loss wearing the other hat.
func TestRevertIsAbandonedOnceTheUserEditsAgain(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(findFixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)
	openFindReplaceBar(t, ctx)
	sendKeysTo(t, ctx, "#find-input", "widget")
	waitForJS(t, ctx, `document.getElementById('find-count').textContent.includes(' of ')`)
	sendKeysTo(t, ctx, "#replace-input", "component")
	evalJS(t, ctx, `(() => { document.getElementById('replace-all').click(); return 'ok' })()`, &res)
	waitForJS(t, ctx, `!/widget/i.test(window.__app.getMarkdown())`)

	// An ordinary edit in the document.
	sendKeysTo(t, ctx, "#raw textarea", " and more")
	if !waitForJS(t, ctx, `window.__app.getMarkdown().includes('and more')`) {
		t.Fatal("the follow-up edit never reached the document")
	}

	evalJS(t, ctx, `document.dispatchEvent(new KeyboardEvent('keydown', {
		key: 'z', metaKey: true, bubbles: true, cancelable: true
	})); 'ok'`, &res)

	var after string
	evalJS(t, ctx, `(async () => { await new Promise((r) => setTimeout(r, 300)); return window.__app.getMarkdown() })()`, &after)
	if !strings.Contains(after, "and more") {
		t.Errorf("reverting after an unrelated edit discarded it: %q", after)
	}
}

// The revert snapshot belongs to ONE document, and pressing Cmd-Z in another
// must not apply it there.
//
// Found by the go/no-go review and reproduced before it was fixed: replace in
// document A, switch to document B, press Cmd-Z, and B's tab held A's
// pre-replace text, marked dirty — so the next save wrote A's content over B's
// file. That is the exact failure internal/session/session.go was written to
// end ("wrote one tab's text over another tab's file"), reintroduced a layer
// above where the session layer cannot see it: by the time Go receives the
// content it is simply what the frontend says the active tab contains, and
// confirmNoExternalChange cannot help because the file on disk still matches
// its baseline. The corruption came from inside the application.
//
// The existing tests could not catch it. They run in raw mode with a single
// document, and the invalidation case only covers a follow-up EDIT — never a
// document switch.
func TestRevertBelongsToTheDocumentItWasTakenFrom(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
	} } ; 'ok'`, &res)

	// Split deliberately: entering split sets the pane authority to 'source',
	// which suppresses the editor's echo through onEdited — and that echo was
	// the only thing incidentally clearing the snapshot on a document change.
	// The mode that made the defect reachable is the mode this test uses.
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote("AAA widget AAA\n")+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	openFindReplaceBar(t, ctx)
	sendKeysTo(t, ctx, "#find-input", "widget")
	waitForJS(t, ctx, `document.getElementById('find-count').textContent.includes(' of ')`)
	sendKeysTo(t, ctx, "#replace-input", "COMPONENT")
	evalJS(t, ctx, `(() => { document.getElementById('replace-all').click(); return 'ok' })()`, &res)
	if !waitForJS(t, ctx, `window.__app.getMarkdown().includes('COMPONENT')`) {
		t.Fatal("replace all did not run in the first document")
	}

	const second = "BBB untouched BBB\n"
	evalJS(t, ctx, `(async () => { await window.__app.newDocument(); return 'ok' })()`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(second)+").then(() => 'ok')", &res)

	evalJS(t, ctx, `document.dispatchEvent(new KeyboardEvent('keydown', {
		key: 'z', metaKey: true, bubbles: true, cancelable: true
	})); 'ok'`, &res)

	var after string
	evalJS(t, ctx, `(async () => { await new Promise((r) => setTimeout(r, 500)); return window.__app.getMarkdown() })()`, &after)
	if after != second {
		t.Errorf("Cmd-Z in a second document applied the first document's snapshot:\n got  %q\n want %q", after, second)
	}

	// And the first document keeps its replacement — abandoning the snapshot
	// must not quietly roll anything back either.
	var docs string
	evalJS(t, ctx, `JSON.stringify(window.__app.state.docs.map((d) => d.markdown))`, &docs)
	if !strings.Contains(docs, "AAA COMPONENT AAA") {
		t.Errorf("the first document lost its replacement: %s", docs)
	}
}
