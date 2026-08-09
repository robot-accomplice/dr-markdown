package e2e

import (
	"testing"
)

// The markdown domain modules are pure — no DOM, no state, no bridge — so they
// can be imported into one already-served page and exercised directly, instead
// of driving the whole app to reach them. Same mechanism as the fidelity units.

func TestTableOperations(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const T = await import('/src/markdown/tables.js')
		const table = '| a | b |\n| --- | --- |\n| 1 | 2 |'
		return [
			String(T.isTableRow('| a | b |')),
			String(T.isDividerRow('| --- | --- |')),
			String(T.containsTable('no table here')),
			String(T.containsTable(table)),
			T.splitTableRow('| a | b |').join(','),
			T.tableRow(['x', 'y']),
			T.tableMarkdown(2, 2),
			T.addTableRow(table),
			T.addTableColumn(table),
			T.alignTable(table, 'center'),
			T.deleteTable(table),
			T.addTableRow('not a table at all'),
		]
	})()`, &got)

	want := []string{
		"true",
		"true",
		"false",
		"true",
		"a,b",
		"| x | y |",
		"| Header 1 | Header 2 |\n| --- | --- |\n| Cell 1.1 | Cell 1.2 |",
		"| a | b |\n| --- | --- |\n| 1 | 2 |\n|  |  |",
		"| a | b | Header 3 |\n| --- | --- | --- |\n| 1 | 2 |  |",
		"| a | b |\n| :---: | :---: |\n| 1 | 2 |",
		"",
		"not a table at all",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestFenceOperations(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const F = await import('/src/markdown/fences.js')
		const js = '` + "```" + `js\nconst a = 1\n` + "```" + `\n'
		const mermaid = '` + "```" + `mermaid\ngraph TD\n` + "```" + `\n'
		return [
			F.firstCodeFenceLanguage(js),
			F.firstCodeFenceLanguage('no fence here'),
			String(F.containsMermaidDiagram(mermaid)),
			String(F.containsMermaidDiagram(js)),
			F.fencedLanguages(js + mermaid).join(','),
			F.rewriteCodeFenceLanguage(js, 0, 'python'),
		]
	})()`, &got)

	// `js` normalizes to `javascript`: a fence's info string and the
	// highlighter's language id have to agree, or the block renders
	// unhighlighted. fencedLanguages reads the raw info string, which is why it
	// still reports `js` while firstCodeFenceLanguage reports `javascript`.
	want := []string{
		"javascript",
		"",
		"true",
		"false",
		"js,mermaid",
		"```python\nconst a = 1\n```\n",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestImageTokens(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const I = await import('/src/markdown/images.js')
		const md = '![alt](a.png)\n\n<img src="b.png" alt="B" width="200">\n'
		const p1 = I.parseImageToken('![alt](a.png)')
		const p2 = I.parseImageToken('<img src="b.png" alt="B" width="200">')
		return [
			String(I.imageTokens(md).length),
			p1.alt + '|' + p1.path + '|' + p1.width,
			p2.alt + '|' + p2.path + '|' + p2.width,
			I.formatImageToken({ alt: 'x', path: 'y.png', width: '' }),
			I.formatImageToken({ alt: 'x', path: 'y.png', width: '300' }),
			String(I.imageTokens('no images').length),
		]
	})()`, &got)

	want := []string{
		"2",
		"alt|a.png|",
		"B|b.png|200",
		"![x](y.png)",
		`<img src="y.png" alt="x" width="300">`,
		"0",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}

// These are the obfuscation cases from TestObfuscatedSchemesAreRefusedInRenderedLinks,
// asserted against the function directly. That test stays exactly as it is — it
// proves the check is WIRED IN, which this one cannot.
func TestLinkSafety(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const L = await import('/src/markdown/links.js')
		const r = (h) => String(L.safeLinkHref(h))
		return [
			r('https://example.com/page'),
			r('mailto:a@b.c'),
			r('notes/other.md'),
			r('javascript:alert(1)'),
			r('jav\tascript:alert(1)'),
			r('JAV\tASCRIPT:alert(1)'),
			r('\x01javascript:alert(1)'),
			r('data:text/html,<b>x</b>'),
			r('vbscript:msgbox(1)'),
			r('file:///etc/passwd'),
		]
	})()`, &got)

	want := []string{
		"https://example.com/page", "mailto:a@b.c", "notes/other.md",
		"null", "null", "null", "null", "null", "null", "null",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestDocumentTextConventions(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const X = await import('/src/markdown/text.js')
		return [
			JSON.stringify(X.detectLineEnding('a\r\nb')),
			JSON.stringify(X.detectLineEnding('a\nb')),
			JSON.stringify(X.toEditorText('a\r\nb')),
			JSON.stringify(X.toFileText('a\nb', '\r\n')),
			JSON.stringify(X.toFileText('a\nb', '\n')),
			X.titleForPath('/tmp/notes/todo.md'),
			X.titleForPath('C:\\docs\\todo.md'),
			X.titleForPath(''),
		]
	})()`, &got)

	want := []string{
		`"\r\n"`, `"\n"`, `"a\nb"`, `"a\r\nb"`, `"a\nb"`,
		"todo.md", "todo.md", "Untitled.md",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}
