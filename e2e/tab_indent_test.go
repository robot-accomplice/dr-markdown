package e2e

import (
	"strconv"
	"testing"
)

// Tab indents by the same amount whichever mode you are in.
//
// Reported from real use: "indenting is different when tab is pressed in
// raw/split versus wysiwyg". Measured on one document — a Go fence with the
// caret at the start of the code line:
//
//	raw        Tab -> "    fmt.Println()"   4 spaces
//	formatted  Tab -> "  fmt.Println()"     2 spaces
//
// Neither value was wrong on its own. Raw mode used the app's own constant and
// the formatted view's code blocks are CodeMirror, whose indentUnit facet
// defaults to two spaces; nothing connected them, so the same keystroke on the
// same document produced different text depending on the mode. A document
// edited in both — which split view makes routine — ended up indented two ways.
//
// This asserts the agreement, not the number: both modes read the width from
// frontend/dist/src/indent.js, and the test reads it from the app too. Changing
// the width is a one-line change that this test follows rather than blocks.
func TestTabIndentsTheSameInEveryMode(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	const doc = "```go\nfmt.Println()\n```\n"

	var width int
	evalJS(t, ctx, `import('./src/indent.js').then((m) => m.INDENT_WIDTH)`, &width)
	if width <= 0 {
		t.Fatalf("could not read INDENT_WIDTH from the app: got %d", width)
	}
	t.Logf("the application's declared indent width is %d", width)

	// Raw mode: a textarea, so the caret and the value are directly readable.
	var rawLine string
	evalJS(t, ctx, `(async () => {
		await window.__app.setMarkdown(`+strconv.Quote(doc)+`)
		window.__app.setMode('raw')
		await new Promise((r) => setTimeout(r, 400))
		const ta = document.querySelector('#raw textarea') || document.querySelector('textarea')
		const i = ta.value.indexOf('fmt.Println')
		ta.focus()
		ta.setSelectionRange(i, i)
		ta.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
		await new Promise((r) => setTimeout(r, 200))
		return ta.value.split('\n').find((l) => l.includes('fmt.Println'))
	})()`, &rawLine)

	// Formatted mode: the same line, inside the code block's CodeMirror.
	var formattedLine string
	evalJS(t, ctx, `(async () => {
		await window.__app.setMarkdown(`+strconv.Quote(doc)+`)
		window.__app.setMode('wysiwyg')
		await new Promise((r) => setTimeout(r, 900))
		const cm = document.querySelector('#wysiwyg .cm-content')
		const line = cm.querySelector('.cm-line')
		const sel = window.getSelection()
		const range = document.createRange()
		range.setStart(line.firstChild || line, 0)
		range.collapse(true)
		sel.removeAllRanges()
		sel.addRange(range)
		cm.focus()
		cm.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
		await new Promise((r) => setTimeout(r, 300))
		return Array.from(cm.querySelectorAll('.cm-line')).map((l) => l.textContent)
			.find((l) => l.includes('fmt.Println'))
	})()`, &formattedLine)

	// The CSS token that sizes a rendered tab character has to agree with the
	// width Tab inserts, or a document containing literal tabs lines up
	// differently from one indented with the app's own key. CSS cannot import
	// indent.js, so the agreement is only ever as good as this assertion.
	var tabSize int
	evalJS(t, ctx, `parseInt(getComputedStyle(document.documentElement)
		.getPropertyValue('--code-tab-size'), 10)`, &tabSize)
	if tabSize != width {
		t.Errorf("--code-tab-size is %d but indent.js declares %d: a literal tab renders at a "+
			"different width from the one Tab inserts", tabSize, width)
	}

	lead := func(s string) int {
		n := 0
		for n < len(s) && s[n] == ' ' {
			n++
		}
		return n
	}
	rawIndent, formattedIndent := lead(rawLine), lead(formattedLine)
	t.Logf("raw:       %q -> %d spaces", rawLine, rawIndent)
	t.Logf("formatted: %q -> %d spaces", formattedLine, formattedIndent)

	if rawIndent != formattedIndent {
		t.Errorf("Tab indents by %d in raw and %d in formatted: the same keystroke on the same "+
			"document gives different text depending on the mode", rawIndent, formattedIndent)
	}
	for _, c := range []struct {
		mode string
		got  int
	}{{"raw", rawIndent}, {"formatted", formattedIndent}} {
		if c.got != width {
			t.Errorf("%s mode indents by %d but the application declares %d in indent.js",
				c.mode, c.got, width)
		}
	}
}
