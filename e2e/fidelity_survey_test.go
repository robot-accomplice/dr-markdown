package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// FIDELITY SURVEY — a wide net over the re-serialization class.
//
// `TestWysiwygRewritesTheseConstructs` records a handful of constructs in
// detail. Its weakness is stated in the release record and is real: it
// converges on "everything we thought to check", so a construct nobody listed
// is rewritten silently. This test widens the map — 49 ordinary CommonMark and
// GFM constructs, each fed through the WYSIWYG surface and compared byte for
// byte — and pins the exact SET that does not survive.
//
// It fails in both directions on purpose. A construct that starts being
// rewritten is a regression; one that stops is a fix that must be recorded here
// and in the README. Asserting the set rather than a count matters: a count
// stays green while one construct breaks and another is fixed.
func TestFidelitySurveyRewritesExactlyTheseConstructs(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	cases := []struct{ name, in string }{
		// headings
		{"atx", "# One\n\n## Two\n"},
		{"atx closed", "# One #\n\n## Two ##\n"},
		{"setext", "One\n===\n\nTwo\n---\n"},
		// emphasis
		{"emph underscore", "_emph_ and __strong__\n"},
		{"emph asterisk", "*emph* and **strong**\n"},
		{"strikethrough", "~~gone~~\n"},
		// breaks
		{"hard break two space", "a  \nb\n"},
		{"hard break backslash", "a\\\nb\n"},
		{"soft break", "a\nb\n"},
		// lists
		{"list dash", "- a\n- b\n"},
		{"list plus", "+ a\n+ b\n"},
		{"ordered paren", "1) a\n2) b\n"},
		{"ordered no increment", "1. a\n1. b\n"},
		{"loose list", "- a\n\n- b\n"},
		{"nested list", "- a\n  - b\n"},
		{"task list", "- [ ] todo\n- [x] done\n"},
		// code
		{"fence backtick", "```js\nconst a = 1\n```\n"},
		{"fence tilde", "~~~js\nconst a = 1\n~~~\n"},
		{"fence info spaces", "``` js\nx\n```\n"},
		{"indented code", "    indented\n"},
		{"inline code", "`code`\n"},
		{"inline code doubled", "``a ` b``\n"},
		// quotes / rules
		{"blockquote", "> quoted\n"},
		{"blockquote nested", "> a\n>\n> > b\n"},
		{"rule dash", "---\n"},
		{"rule underscore", "___\n"},
		// links / images
		{"inline link", "[a](http://x)\n"},
		{"link title", "[a](http://x \"t\")\n"},
		{"autolink", "<http://x>\n"},
		{"bare url", "http://x\n"},
		{"ref link", "[a][r]\n\n[r]: http://x\n"},
		{"collapsed ref", "[a][]\n\n[a]: http://x\n"},
		{"shortcut ref", "[a]\n\n[a]: http://x\n"},
		{"image", "![alt](i.png)\n"},
		// html
		{"inline html", "a <b>bold</b> c\n"},
		{"html block", "<div>\n  <p>x</p>\n</div>\n"},
		{"html comment", "<!-- note -->\n"},
		// tables
		{"table", "| a | b |\n| --- | --- |\n| 1 | 2 |\n"},
		{"table aligned", "| a | b |\n| :-- | --: |\n| 1 | 2 |\n"},
		{"table unpadded", "|a|b|\n|-|-|\n|1|2|\n"},
		// escapes / entities / misc
		{"escaped char", "\\*not emph\\*\n"},
		{"entity", "&amp; &copy;\n"},
		{"footnote", "a[^1]\n\n[^1]: note\n"},
		{"trailing spaces", "text   \n"},
		{"multiple blank lines", "a\n\n\n\nb\n"},
		{"tab indent list", "-\ta\n"},
		{"unicode", "café — ünïcode 中文\n"},
		{"emphasis mid word", "a*b*c\n"},
		{"numbered start", "3. a\n4. b\n"},
	}

	type result struct{ Name, In, Got string }
	var payload []map[string]string
	for _, c := range cases {
		payload = append(payload, map[string]string{"name": c.name, "in": c.in})
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	var out []result
	evalJS(t, ctx, `(async () => {
	  const cases = `+string(blob)+`
	  const out = []
	  for (const c of cases) {
	    await window.__app.setMarkdown(c.in)
	    const got = window.__app.getEditorMarkdown()
	    out.push({ Name: c.name, In: c.in, Got: got })
	  }
	  return out
	})()`, &out)

	// The constructs measured as rewritten on the current build. Every entry is
	// observed, not predicted, and each is disclosed in the README caution.
	rewritten := map[string]bool{
		"hard break two space": true, // `a  \nb` → `a\\\nb`; no serializer option expresses it
		"fence info spaces":    true, // "``` js" → "```js"
		"indented code":        true, // four-space block → fenced block
		"bare url":             true, // `http://x` → `<http://x>`
		"table":                true, // delimiter row padding recomputed from content
		"table aligned":        true, // as above, plus alignment colons repadded
		"table unpadded":       true, // `|a|b|` → `| a | b |`
		"entity":               true, // `&amp; &copy;` → `& ©`
		"trailing spaces":      true, // insignificant trailing whitespace dropped
		"multiple blank lines": true, // runs of blank lines collapsed to one
		"tab indent list":      true, // tab after the marker → space (marker itself survives)
	}

	var unexpected, fixed []result
	for _, r := range out {
		changed := r.Got != r.In
		if changed && !rewritten[r.Name] {
			unexpected = append(unexpected, r)
		}
		if !changed && rewritten[r.Name] {
			fixed = append(fixed, r)
		}
	}
	if len(out) != len(cases) {
		t.Fatalf("survey returned %d results for %d cases", len(out), len(cases))
	}

	for _, r := range unexpected {
		t.Errorf("REGRESSION — %q is newly rewritten:\n in:  %q\n got: %q", r.Name, r.In, r.Got)
	}
	for _, r := range fixed {
		t.Errorf("IMPROVEMENT — %q now round-trips exactly. Remove it from the rewritten set "+
			"here and from the README caution, so the disclosure cannot drift from behaviour.", r.Name)
	}

	if len(unexpected) == 0 && len(fixed) == 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%d/%d constructs round-trip byte-identically; %d rewritten as recorded",
			len(out)-len(rewritten), len(out), len(rewritten))
		t.Log(b.String())
	}
}
