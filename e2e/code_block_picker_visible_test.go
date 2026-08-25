package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// The in-block language picker must be VISIBLE, not merely present and wired.
//
// TestExistingCodeBlockLanguageCanBeChangedFromTheBlockPicker drives this same
// picker and passes, because it clicks a list item through the DOM and asserts
// the resulting fence. That is the #77 shape again one level up: a synthetic
// click reaches a node the block has clipped to 11% of its height, so the
// capability is provably functional and invisible to a person at the same time.
// The app's own surface rule was the clip — `overflow: hidden` copied from
// `.code-block-shell`, which needs it, onto the editor's block, which contains
// the picker (#80).
//
// So this gate measures geometry and hit-testing rather than behaviour. It is
// the only kind of check that can fail for a clipped or buried dropdown.
func TestCodeBlockLanguagePickerIsVisibleOutsideTheBlock(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{"# Picker", "", "```python", "print(1)", "```", ""}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .language-button') !== null`) {
		t.Fatal("the code block should expose the editor's language picker")
	}
	evalJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .language-button').click(); 'ok'`, &res)

	var verdict string
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 400))
		const block = document.querySelector('#wysiwyg .milkdown-code-block')
		const wrap = block && block.querySelector('.list-wrapper')
		if (!wrap) return 'FAIL: the picker did not open'

		const b = block.getBoundingClientRect()
		const w = wrap.getBoundingClientRect()

		// The dropdown is taller than the block it hangs off, so it MUST extend
		// past the block's box. That is the point of the fixture: if it does not
		// overflow, the assertion below proves nothing.
		if (w.height <= b.height) {
			return 'INCONCLUSIVE: dropdown (' + Math.round(w.height) +
				'px) is not taller than the block (' + Math.round(b.height) + 'px), so this ' +
				'fixture cannot detect clipping — give the picker a taller list or the block a shorter body'
		}

		// Hit-test three points down the dropdown, INCLUDING one below the
		// block's own bottom edge. A clipping ancestor fails the last one.
		const cx = w.left + w.width / 2
		const points = [
			['top', w.top + 8],
			['middle', w.top + w.height / 2],
			['below the block', Math.min(b.bottom + 24, w.bottom - 8)],
		]
		const missed = []
		for (const [name, y] of points) {
			const hit = document.elementFromPoint(Math.round(cx), Math.round(y))
			if (!hit || !wrap.contains(hit)) {
				missed.push(name + ' (hit ' + (hit ? (hit.className || hit.tagName) : 'nothing') + ')')
			}
		}
		if (missed.length) {
			return 'FAIL: the dropdown is not hit-testable at: ' + missed.join(', ') +
				'; block overflow=' + getComputedStyle(block).overflow +
				', picker z-index=' + getComputedStyle(block.querySelector('.language-picker')).zIndex
		}
		return 'ok'
	})()`, &verdict)

	if verdict != "ok" {
		t.Error(verdict)
	}
}

// The picker must use the app's tokens, not the vendored Crepe palette.
//
// #79 cancelled the vendored warm tint on `.cm-editor` and `.cm-gutters`, but
// the picker is a sibling of the CodeMirror host rather than a descendant, so
// it kept rendering in peach — measured rgb(255, 241, 229) — inside an app
// surface (#80). A colour assertion is worth having because the vendored rules
// out-specify a careless override: they are (0,3,0), and `.milkdown *` alone
// already beat an app rule once (#76).
func TestCodeBlockLanguagePickerUsesTheAppSurface(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{"# Picker", "", "```python", "print(1)", "```", ""}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .language-button') !== null`) {
		t.Fatal("the code block should expose the editor's language picker")
	}
	evalJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .language-button').click(); 'ok'`, &res)

	var verdict string
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 400))
		const block = document.querySelector('#wysiwyg .milkdown-code-block')
		const wrap = block && block.querySelector('.list-wrapper')
		if (!wrap) return 'FAIL: the picker did not open'

		// The document body carries the app's own surface, so comparing against
		// it keeps this gate theme-agnostic: it must hold in dark mode too,
		// where the vendored peach would be doubly wrong.
		const panel = getComputedStyle(document.documentElement).getPropertyValue('--panel').trim()
		const probe = document.createElement('div')
		probe.style.background = panel
		document.body.appendChild(probe)
		const want = getComputedStyle(probe).backgroundColor
		probe.remove()

		const got = getComputedStyle(wrap).backgroundColor
		if (got !== want) {
			return 'FAIL: dropdown background is ' + got + ', but the app surface --panel is ' + want
		}
		return 'ok'
	})()`, &verdict)

	if verdict != "ok" {
		t.Error(verdict)
	}
}
