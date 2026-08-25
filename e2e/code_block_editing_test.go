package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// WYSIWYG is the defining purpose of this editor: everything that renders in
// Formatted mode must be editable there. A construct that renders but cannot be
// edited is a defect regardless of what any screen inventory says about it
// (docs/architext/data/rules.json, `wysiwyg-is-the-purpose`).
//
// Code blocks failed that rule from the day syntax highlighting landed, and
// shipped broken in v0.5.1, because no test had ever typed into one. The
// existing chrome test asserts a language label and a Copy button exist, and it
// passed for the whole time the block underneath them was inert — presence
// cannot fail for an editability fault.
//
// So this gate types. It clicks into the rendered block, sends real keystrokes
// through the browser's own input path, and requires the characters to come
// back out of the document's markdown. Nothing short of a genuinely editable
// surface can satisfy it.
func TestCodeBlockBodyIsEditableInFormattedMode(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	const fixture = "```go\nfunc main() {}\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	// The editable surface is CodeMirror's content element, mounted by Crepe's
	// code-mirror feature when the block scrolls into view.
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .cm-content[contenteditable="true"]') !== null`) {
		t.Fatal("no editable surface inside the rendered code block: the block renders " +
			"but cannot be typed into, which is exactly the defect this gate exists to catch")
	}

	const typed = "ZZTOP"
	clickWhenVisible(t, ctx, "#wysiwyg .cm-content")
	sendKeysTo(t, ctx, "#wysiwyg .cm-content", typed)

	var got string
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 500))
		return window.__app.getMarkdown()
	})()`, &got)

	if !strings.Contains(got, typed) {
		t.Errorf("typed %q into the code block but the document's markdown is:\n%s\n"+
			"the keystrokes never reached the document", typed, got)
	}
	// The body must still be a fenced code block, not text that escaped it.
	if !strings.Contains(got, "```go") {
		t.Errorf("the code fence did not survive editing, markdown is:\n%s", got)
	}
}
