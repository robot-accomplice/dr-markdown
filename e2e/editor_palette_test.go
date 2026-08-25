package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// No vendored warm value may reach the screen, anywhere in the editor.
//
// `vendor/theme/crepe/style.css` is a brown/peach Material palette, and every
// in-block control draws from it: the language picker and its list, the copy and
// preview buttons, CodeMirror's search panel, the placeholder, and the outline
// on a selected block. Two rules had already cancelled two of those tints by
// hand (the inline-code red on `pre code`, and the editor surface), and the
// picker still shipped peach anyway (#80) — which is the argument against
// fixing this one selector at a time. The palette is rebound at its source, and
// this gate is what makes that hold as new vendored controls appear.
//
// It fails on the VALUES, not on the rules, so it cannot be satisfied by a
// selector that merely looks correct.
func TestEditorUsesNoVendoredWarmColour(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	// Every construct that owns in-block controls, so the walk below has
	// something of each kind to inspect.
	fixture := strings.Join([]string{
		"# Palette",
		"",
		"Paragraph with `inline code` in it.",
		"",
		"```python",
		"print('hello')",
		"```",
		"",
		"```mermaid",
		"graph TD",
		"  A[Start] --> B[End]",
		"```",
		"",
		"| a | b |",
		"| --- | --- |",
		"| 1 | 2 |",
		"",
	}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .language-button') !== null`) {
		t.Fatal("the fixture should render a code block with the editor's controls")
	}
	// Open the picker so its list is in the tree while the walk runs.
	evalJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .language-button').click(); 'ok'`, &res)

	var verdict string
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 400))

		// The vendored palette, verbatim from vendor/theme/crepe/style.css.
		const banned = {
			'rgb(255, 253, 251)': '--crepe-color-background #fffdfb',
			'rgb(255, 248, 244)': '--crepe-color-surface #fff8f4',
			'rgb(255, 241, 229)': '--crepe-color-surface-low #fff1e5',
			'rgb(249, 236, 223)': '--crepe-color-hover #f9ecdf',
			'rgb(237, 224, 212)': '--crepe-color-selected #ede0d4',
			'rgb(251, 222, 188)': '--crepe-color-secondary #fbdebc',
			'rgb(128, 86, 16)':   '--crepe-color-primary #805610',
			'rgb(129, 117, 103)': '--crepe-color-outline #817567',
			'rgb(228, 216, 204)': '--crepe-color-inline-area #e4d8cc',
			'rgb(252, 239, 226)': '--crepe-color-on-inverse #fcefe2',
			'rgb(79, 69, 57)':    '--crepe-color-on-surface-variant #4f4539',
			'rgb(32, 27, 19)':    '--crepe-color-on-surface #201b13',
			'rgb(31, 27, 22)':    '--crepe-color-on-background #1f1b16',
			'rgb(39, 25, 4)':     '--crepe-color-on-secondary #271904',
			'rgb(54, 47, 39)':    '--crepe-color-inverse #362f27',
		}
		const props = ['backgroundColor', 'color', 'borderTopColor', 'borderBottomColor',
			'borderLeftColor', 'borderRightColor', 'outlineColor', 'caretColor']

		const root = document.querySelector('#wysiwyg .milkdown')
		if (!root) return 'FAIL: no .milkdown root to inspect'

		const hits = []
		for (const el of [root, ...root.querySelectorAll('*')]) {
			const cs = getComputedStyle(el)
			for (const p of props) {
				const v = cs[p]
				if (banned[v]) {
					const where = el.tagName.toLowerCase() +
						(el.className && typeof el.className === 'string' ? '.' + el.className.trim().split(/\s+/).join('.') : '')
					hits.push(where + ' ' + p + '=' + v + '  (' + banned[v] + ')')
				}
			}
			if (hits.length >= 8) break
		}
		if (hits.length) {
			return 'FAIL: vendored warm palette reaching the screen:\n  ' + hits.join('\n  ')
		}
		return 'ok'
	})()`, &verdict)

	if verdict != "ok" {
		t.Error(verdict)
	}
}

// The remap must be a REMAP, not a repaint: the vendored variables themselves
// have to resolve to this app's tokens, so a vendored rule nobody has looked at
// is correct before anyone looks at it. Asserting the variables (rather than
// some element's colour) is what distinguishes the two.
func TestVendoredPaletteIsBoundToAppTokens(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{"# Palette", "", "```python", "print(1)", "```", ""}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .milkdown') !== null`) {
		t.Fatal("no editor root")
	}

	var verdict string
	evalJS(t, ctx, `(() => {
		const root = document.querySelector('#wysiwyg .milkdown')
		const cs = getComputedStyle(root)
		const app = getComputedStyle(document.documentElement)

		// Resolve a token through a probe so both sides are compared as the
		// browser sees them, not as authored text.
		const resolve = (value) => {
			const probe = document.createElement('div')
			probe.style.color = value
			document.body.appendChild(probe)
			const out = getComputedStyle(probe).color
			probe.remove()
			return out
		}

		// The bindings that matter: the surfaces the controls sit on, the
		// primary that draws focus rings and the selected outline, and the
		// hover the lists use. Teal, not brown.
		const want = [
			['--crepe-color-surface', '--panel'],
			['--crepe-color-surface-low', '--panel'],
			['--crepe-color-primary', '--accent'],
			['--crepe-color-hover', '--accent-wash'],
			['--crepe-color-on-surface', '--ink'],
		]
		const bad = []
		for (const [crepe, token] of want) {
			const got = resolve(cs.getPropertyValue(crepe).trim())
			const expected = resolve(app.getPropertyValue(token).trim())
			if (got !== expected) bad.push(crepe + ' resolves to ' + got + ', expected ' + token + ' = ' + expected)
		}
		if (bad.length) return 'FAIL:\n  ' + bad.join('\n  ')
		return 'ok'
	})()`, &verdict)

	if verdict != "ok" {
		t.Error(verdict)
	}
}
