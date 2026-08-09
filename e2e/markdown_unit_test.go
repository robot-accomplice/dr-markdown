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
