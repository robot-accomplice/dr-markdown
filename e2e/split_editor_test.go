package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// Split's formatted pane is the REAL editor, not a rendering of the document.
//
// It used to be a preview drawn by a second renderer, and that second renderer
// was the whole of #134: it disagreed with the editor about sixteen ordinary
// constructs, and it degraded further every time the editor gained a feature it
// did not have. Two renderers for one document is the defect; one editor shown
// in two places is the fix, so the surfaces cannot drift apart again because
// there is only one of them.
//
// The editor ELEMENT is moved between the document card and the split pane. The
// instance is never rebuilt to move it, which is what keeps the document,
// undo history and mounted node views intact across a mode switch.
func TestSplitFormattedPaneIsTheRealEditor(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote("# Heading\n\ntext\n")+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	var inPane, editable bool
	evalJS(t, ctx, `document.querySelector('[data-split-pane="formatted"]').contains(document.getElementById('wysiwyg'))`, &inPane)
	if !inPane {
		t.Fatal("split's formatted pane should host the editor itself")
	}
	evalJS(t, ctx, `document.querySelector('[data-split-pane="formatted"] #wysiwyg [contenteditable="true"]') !== null`, &editable)
	if !editable {
		t.Error("the formatted pane must be editable; a read-only pane is the preview this replaced")
	}

	// Leaving split must put the editor back in the document card. If it did
	// not, the card would be empty in formatted mode — and because the element
	// is moved rather than rebuilt, nothing else would report a failure.
	evalJS(t, ctx, "window.__app.setMode('wysiwyg').then(() => 'ok')", &res)
	var returned string
	evalJS(t, ctx, `document.getElementById('wysiwyg').parentElement?.id ?? ''`, &returned)
	if returned != "editor-host" {
		t.Errorf("editor did not return to the document card; parent = %q", returned)
	}
	var stillHasContent bool
	evalJS(t, ctx, `document.getElementById('wysiwyg').textContent.includes('Heading')`, &stillHasContent)
	if !stillHasContent {
		t.Error("the document did not survive the move out of the split pane")
	}
}

// Both panes are live editors over one document, so an edit in either must
// reach the other. This is driven with real key events rather than by setting
// values and dispatching synthetic ones: the pane that is authoritative is
// decided by keydown and pointerdown, and a synthetic input event would take a
// path no user can take. The first implementation keyed on focus instead and
// passed a synthetic test while being broken in the app — the editor already
// holds focus when split opens, so the formatted pane could never become
// authoritative and typing there never reached the source pane.
func TestSplitPanesStayInSyncInBothDirections(t *testing.T) {
	t.Run("formatted to source", func(t *testing.T) {
		ctx, cancel := newTestBrowser(t)
		defer cancel()
		url := serveFrontend(t)
		bootApp(t, ctx, url)

		var res string
		evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote("start\n")+").then(() => 'ok')", &res)
		evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

		sendKeysTo(t, ctx, "#wysiwyg [contenteditable='true']", "ABC")
		if !waitForJS(t, ctx, `document.getElementById('split-source').value.includes('ABC')`) {
			var got string
			evalJS(t, ctx, `document.getElementById('split-source').value`, &got)
			t.Fatalf("an edit in the formatted pane never reached the source pane; source = %q", got)
		}

		// The document itself must agree, not just the other pane.
		var md string
		evalJS(t, ctx, `window.__app.getMarkdown()`, &md)
		if !strings.Contains(md, "ABC") {
			t.Errorf("the document did not record an edit made in the formatted pane; got %q", md)
		}
	})

	t.Run("source to formatted", func(t *testing.T) {
		ctx, cancel := newTestBrowser(t)
		defer cancel()
		url := serveFrontend(t)
		bootApp(t, ctx, url)

		var res string
		evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote("start\n")+").then(() => 'ok')", &res)
		evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

		sendKeysTo(t, ctx, "#split-source", " XYZ")
		if !waitForJS(t, ctx, `document.getElementById('wysiwyg').textContent.includes('XYZ')`) {
			t.Fatal("an edit in the source pane never reached the formatted pane")
		}

		var md string
		evalJS(t, ctx, `window.__app.getMarkdown()`, &md)
		if !strings.Contains(md, "XYZ") {
			t.Errorf("the document did not record an edit made in the source pane; got %q", md)
		}
	})
}

// The formatted pane rebuilds itself from the source pane after a pause in
// typing. That rebuild serializes its content and reports it as a change, which
// is indistinguishable from a real edit unless something says otherwise — and
// letting that echo through would write the rebuilt text back over whatever the
// user has typed since, deleting keystrokes mid-sentence.
func TestTypingInTheSourcePaneIsNotOverwrittenByTheFormattedPane(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote("start\n")+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	// Type, pause long enough for the rebuild to run, then keep typing. The
	// pause is the point: without it the rebuild never fires and the test
	// cannot fail.
	sendKeysTo(t, ctx, "#split-source", " one")
	if !waitForJS(t, ctx, `document.getElementById('wysiwyg').textContent.includes('one')`) {
		t.Fatal("the formatted pane never rebuilt from the source pane")
	}
	sendKeysTo(t, ctx, "#split-source", " two")

	var source string
	evalJS(t, ctx, `document.getElementById('split-source').value`, &source)
	if !strings.Contains(source, "one") || !strings.Contains(source, "two") {
		t.Errorf("the formatted pane's rebuild clobbered typing in the source pane; source = %q", source)
	}
}
