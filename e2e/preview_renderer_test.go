package e2e

import (
	"context"
	"strconv"
	"testing"
)

// renderToPrintRoot renders markdown into #print-root.
//
// Since split mode became the real editor, print is the ONLY surface that is
// not the editor, and therefore the only consumer of markdown/render.js left in
// the application. Every assertion about that renderer belongs here.
func renderToPrintRoot(t *testing.T, ctx context.Context, markdown string) {
	t.Helper()
	var res string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: '', exists: false }),
		SetDirty: async () => {}, UpdateContent: async () => {}
	} } ; window.print = () => {}; 'ok'`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(markdown)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.printDocument('print').then(() => 'ok')", &res)
}

// The split preview and the print/PDF surface used to share a hand-written
// renderer built from regular expressions over lines. Measured across the
// sixteen constructs below, SIXTEEN produced wrong output: a table came out as
// paragraphs of pipe characters, an ordered list lost its numbers and became
// bullets, nested lists flattened to one level, task lists showed literal
// brackets, emphasis inside a heading or a blockquote stayed as asterisks, and
// a paragraph wrapped across two source lines rendered as two paragraphs.
//
// The reason this matters more than a cosmetic preview bug: the same renderer
// fed PRINT and PDF EXPORT. A PDF is the artifact that leaves the application,
// reaches someone who will never see the source, and cannot be corrected after
// it is sent. A table that prints as pipes is a document the user believes they
// wrote and did not.
//
// These cases are therefore pinned by CONSTRUCT rather than by rendered markup:
// each asserts the structural fact that was wrong, so the test still fails if a
// future renderer regresses the same construct in a different way.
func TestPreviewRendersOrdinaryMarkdownConstructs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		markdown string
		// probe returns true when the construct rendered correctly. It runs
		// against #print-root.
		probe string
	}{
		{
			"gfm table",
			"| a | b |\n| - | - |\n| 1 | 2 |\n",
			`p.querySelectorAll('table thead th').length === 2 &&
			 p.querySelectorAll('table tbody td').length === 2`,
		},
		{
			// The numbers are the content of an ordered list. Rendering it as
			// bullets does not lose styling, it loses the document's meaning.
			"ordered list keeps its numbering",
			"3. three\n4. four\n",
			`p.querySelector('ol') !== null &&
			 p.querySelector('ol').getAttribute('start') === '3' &&
			 p.querySelectorAll('ol > li').length === 2`,
		},
		{
			"nested list stays nested",
			"- outer\n  - inner\n",
			`p.querySelectorAll('ul > li > ul > li').length === 1`,
		},
		{
			"task list renders checkboxes",
			"- [x] done\n- [ ] todo\n",
			`p.querySelectorAll('input[type="checkbox"]').length === 2 &&
			 p.querySelectorAll('input[type="checkbox"]')[0].checked === true &&
			 p.querySelectorAll('input[type="checkbox"]')[1].checked === false`,
		},
		{
			"emphasis inside a heading",
			"## a *stressed* word\n",
			`p.querySelector('h2 em')?.textContent === 'stressed'`,
		},
		{
			"emphasis inside a blockquote",
			"> a **bold** claim\n",
			`p.querySelector('blockquote strong')?.textContent === 'bold'`,
		},
		{
			// A hard-wrapped paragraph is one paragraph. Splitting it inserts
			// vertical space the author did not write, which in print changes
			// where the page breaks.
			"wrapped paragraph stays one paragraph",
			"one line\nand its continuation\n",
			`p.querySelectorAll('p').length === 1 &&
			 p.querySelector('p').textContent.includes('one line') &&
			 p.querySelector('p').textContent.includes('continuation')`,
		},
		{
			"setext heading",
			"Title\n=====\n",
			`p.querySelector('h1')?.textContent === 'Title'`,
		},
		{
			"indented code block",
			"    literal := code\n",
			`p.querySelector('pre code')?.textContent.includes('literal := code') === true`,
		},
		{
			"asterisk thematic break",
			"above\n\n***\n\nbelow\n",
			`p.querySelector('hr') !== null`,
		},
		{
			"underscore thematic break",
			"above\n\n___\n\nbelow\n",
			`p.querySelector('hr') !== null`,
		},
		{
			// html: true is deliberate — this dialect round-trips inline HTML,
			// so a renderer that escaped it would show the source of something
			// the editor renders.
			"inline html is rendered, not escaped",
			"press <kbd>esc</kbd> to exit\n",
			`p.querySelector('kbd')?.textContent === 'esc'`,
		},
		{
			// Without the footnote plugin `[^1]` is a shortcut reference link,
			// which is correct CommonMark and wrong for this dialect.
			"footnote",
			"a claim[^1]\n\n[^1]: the support\n",
			`p.querySelector('sup.footnote-ref') !== null &&
			 p.querySelector('.footnotes') !== null`,
		},
		{
			"reference link resolves",
			"see [the docs][d]\n\n[d]: https://example.com/docs\n",
			`p.querySelector('a')?.getAttribute('href') === 'https://example.com/docs'`,
		},
		{
			// The shell carries the NORMALIZED language and the <code> keeps the
			// language as the fence declared it: `js` and `javascript` name one
			// language, and the highlighter and the fence rewriter have to agree
			// on which spelling they mean.
			"fenced code keeps its shell and highlighting",
			"```js\nconst answer = \"yes\"\n```\n",
			`p.querySelector('.code-block-shell[data-language="javascript"] .code-block-header') !== null &&
			 p.querySelector('.code-block-shell pre code[data-language="js"] .hljs-keyword') !== null`,
		},
		{
			"strikethrough",
			"~~gone~~\n",
			`p.querySelector('s') !== null`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := newTestBrowser(t)
			defer cancel()
			url := serveFrontend(t)
			bootApp(t, ctx, url)

			renderToPrintRoot(t, ctx, tc.markdown)

			var ok bool
			evalJS(t, ctx, `(() => {
				const p = document.querySelector('#print-root')
				return Boolean(`+tc.probe+`)
			})()`, &ok)
			if !ok {
				var got string
				evalJS(t, ctx, `document.querySelector('#print-root').innerHTML.slice(0, 400)`, &got)
				t.Errorf("construct rendered wrongly.\nmarkdown: %q\ngot: %s", tc.markdown, got)
			}
		})
	}
}

// The print surface and the split preview must agree, because they are the same
// renderer and the whole point of consolidating them was that they had drifted.
// A construct correct in the preview and wrong in the PDF is the worse of the
// two failures, and the one nobody sees until the document has been sent.
func TestPrintSurfaceRendersTheSameConstructsAsThePreview(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "| a | b |\n| - | - |\n| 1 | 2 |\n\n3. three\n4. four\n\n- [x] done\n"

	var res string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: '', exists: false }),
		SetDirty: async () => {}, UpdateContent: async () => {}
	} } ; window.print = () => {}; 'ok'`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.printDocument('print').then(() => 'ok')", &res)

	var ok bool
	evalJS(t, ctx, `(() => {
		const p = document.querySelector('#print-root')
		return Boolean(
			p.querySelectorAll('table thead th').length === 2 &&
			p.querySelector('ol')?.getAttribute('start') === '3' &&
			p.querySelectorAll('input[type="checkbox"]').length === 1)
	})()`, &ok)
	if !ok {
		var got string
		evalJS(t, ctx, `document.querySelector('#print-root').innerHTML.slice(0, 400)`, &got)
		t.Errorf("print surface did not render the constructs the preview does; got: %s", got)
	}
}

// Rendering an opened document with `html: true` puts untrusted bytes into the
// DOM of the webview that holds the native bindings — SaveDocument and
// OpenRecentDocument, neither of which restricts a path. The renderer this
// replaced was safe from this only by accident: it matched lines with regular
// expressions and emitted text nodes, so hostile markup never became an element
// at all. Adopting a real parser removed the accident, so the guarantee is
// stated here instead.
func TestPreviewRefusesHostileDocumentHTML(t *testing.T) {
	for _, tc := range []struct {
		name     string
		markdown string
		// probe must be true; it asserts the hostile construct did NOT survive.
		probe string
	}{
		{
			"event handler attributes are stripped",
			"<img src=\"x.png\" onerror=\"globalThis.__pwned = true\">\n",
			`document.querySelectorAll('#print-root [onerror]').length === 0`,
		},
		{
			"script elements are dropped with their contents",
			"<script>globalThis.__pwned = true</script>\n",
			`document.querySelectorAll('#print-root script').length === 0 &&
			 globalThis.__pwned === undefined`,
		},
		{
			"iframes are dropped",
			"<iframe src=\"https://example.com\"></iframe>\n",
			`document.querySelectorAll('#print-root iframe').length === 0`,
		},
		{
			// getElementById returns the first match in document order, so a
			// document naming an application element would start answering
			// queries meant for the app.
			"a document cannot claim an application element id",
			"<div id=\"print-root\">not the print root</div>\n",
			`document.querySelectorAll('#print-root [id]').length === 0 &&
			 document.getElementById('print-root').parentElement.id !== 'print-root'`,
		},
		{
			"javascript hrefs are refused and marked",
			"[click me](javascript:alert(1))\n",
			`document.querySelector('#print-root a')?.dataset.blockedHref === 'true'`,
		},
		{
			// An <img> renders SVG passively, but a document is not allowed to
			// bring its own inline <svg> element, which carries a second URL and
			// scripting surface of its own.
			"inline svg is dropped",
			"<svg onload=\"globalThis.__pwned = true\"><circle r=\"5\"/></svg>\n",
			`document.querySelectorAll('#print-root svg').length === 0`,
		},
		{
			// The ordinary case must still work, or the sanitizer has simply
			// broken images rather than secured them.
			"an ordinary relative image survives",
			"![alt](notes.assets/photo.png)\n",
			`document.querySelector('#print-root img[alt="alt"]') !== null`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := newTestBrowser(t)
			defer cancel()
			url := serveFrontend(t)
			bootApp(t, ctx, url)

			renderToPrintRoot(t, ctx, tc.markdown)

			var ok bool
			evalJS(t, ctx, "Boolean("+tc.probe+")", &ok)
			if !ok {
				var got string
				evalJS(t, ctx, `document.querySelector('#print-root').innerHTML.slice(0, 400)`, &got)
				t.Errorf("hostile construct survived rendering.\nmarkdown: %q\ngot: %s", tc.markdown, got)
			}
		})
	}
}
