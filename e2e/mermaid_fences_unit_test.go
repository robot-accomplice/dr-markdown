package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// Mermaid diagrams are drawn before the editor mounts, so the code-block
// preview hook in editor.js can answer synchronously from cache rather than
// filling an element the node view has already copied. That needs the SOURCE of
// every mermaid fence — a different job from fencedLanguages, which only names
// them — and it must skip non-mermaid fences, including one that merely
// contains the word.
//
// The fixture is built in Go rather than inside the evaluated string: a Go raw
// string cannot contain a backtick, and code fences are made of them.
func TestMermaidFenceSources(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	doc := strings.Join([]string{
		"# Title",
		"",
		"```js",
		"const mermaid = 1",
		"```",
		"",
		"```mermaid",
		"graph TD",
		"  A --> B",
		"```",
		"",
		"```MERMAID",
		"sequenceDiagram",
		"```",
		"",
	}, "\n")

	var got []string
	evalJS(t, ctx, `(async () => {
		const F = await import('/src/markdown/fences.js')
		return [
			JSON.stringify(F.mermaidFenceSources(`+strconv.Quote(doc)+`)),
			JSON.stringify(F.mermaidFenceSources('no fences here')),
			JSON.stringify(F.mermaidFenceSources(`+strconv.Quote("```mermaid\n```\n")+`)),
		]
	})()`, &got)

	want := []string{
		`["graph TD\n  A --> B","sequenceDiagram"]`,
		`[]`,
		`[""]`,
	}
	if len(got) != len(want) {
		t.Fatalf("mermaidFenceSources returned %d results, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %s, want %s", i, got[i], want[i])
		}
	}
}
