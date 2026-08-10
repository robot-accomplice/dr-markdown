package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A realistic document loses exactly one thing, and this pins which.
//
// The corpus is otherwise atomic by design — one construct per fixture, four to
// sixteen lines — which localises a failure but cannot see anything that
// happens BETWEEN constructs. 23-realistic-document.md is 86 lines shaped like
// a document somebody would actually write, and on its first run it found
// something twenty-two atomic fixtures had not: a link reference definition
// preceded by other content comes back with an extra blank line before it.
//
// 17-link-refs.canonical.md cannot catch that. It is nothing but definitions,
// so there is no preceding content for the appended block to be separated from.
//
// Recorded in BOTH directions, like the fidelity gates: if this stops
// happening, the test goes red and the fix must be disclosed rather than
// landing silently.
func TestRealisticDocumentLosesOnlyTheLinkRefBlankLine(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	path := filepath.Join("..", "testdata", "roundtrip", "23-realistic-document.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	input := string(data)
	inJSON, _ := json.Marshal(input)

	var output string
	evalJS(t, ctx,
		"window.__app.setMarkdown("+string(inJSON)+").then(() => window.__app.getEditorMarkdown())",
		&output)

	if output == input {
		t.Fatal("the realistic document now round-trips byte-identically. " +
			"That is a FIX: rename it to 23-realistic-document.canonical.md, " +
			"update testdata/README.md, and delete this test.")
	}

	// Everything except the blank line before the definition block must survive.
	// Comparing with blank lines dropped isolates that one difference: if
	// anything else changed — a fence, a tab inside one, a table cell, the
	// blockquote — this fails and names it.
	if got, want := withoutBlankLines(output), withoutBlankLines(input); got != want {
		t.Errorf("a realistic document lost more than the link-ref blank line.\n"+
			"--- first differing line ---\n%s", firstDifference(want, got))
	}

	// And the deviation must stay the one we think it is.
	if strings.Count(output, "\n") != strings.Count(input, "\n")+1 {
		t.Errorf("expected exactly one added line, got %d in and %d out",
			strings.Count(input, "\n"), strings.Count(output, "\n"))
	}
}

func withoutBlankLines(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func firstDifference(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			return "line " + itoa(i+1) + "\n  want: " + a + "\n  got : " + b
		}
	}
	return "(no difference outside blank lines)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
