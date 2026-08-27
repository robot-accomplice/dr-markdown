package e2e

import (
	"testing"
)

// markdown/search.js is pure — offsets into a string, no DOM — so it is
// exercised directly rather than through the app, like the other domain units.
//
// Matches are offsets into the MARKDOWN SOURCE because that is the one
// coordinate all three modes share. A search that ran over whichever surface
// was visible would answer differently in Raw, Formatted and Split, and in
// Split it would answer differently from itself.
func TestDocumentSearchFindsSourceRanges(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const S = await import('/src/markdown/search.js')
		const out = []
		const spans = (text, q, o) => S.findMatches(text, q, o).map((m) => m.start + '-' + m.end).join(',')

		// Plain matching, and the count is what a match count means.
		out.push(spans('one two one', 'one'))
		out.push(String(S.findMatches('one two one', 'one').length))

		// Case folding is the default; the toggle turns it off.
		out.push(spans('One one ONE', 'one'))
		out.push(spans('One one ONE', 'one', { caseSensitive: true }))

		// A literal search must not be read as a pattern, or searching for a
		// dollar sign or a bracket is a syntax error rather than a search.
		out.push(spans('a.b axb', 'a.b'))
		out.push(spans('cost (net)', '(net)'))

		// Regex mode is opt-in.
		out.push(spans('a1 b2 c3', '[a-z]\\d', { regex: true }))

		// Whole word.
		out.push(spans('cat cats concat', 'cat', { wholeWord: true }))

		// An empty query is not a match on every position.
		out.push(String(S.findMatches('anything', '').length))

		// A pattern that matches the empty string must terminate. Without the
		// advance it loops forever and the browser hangs on a keystroke.
		out.push(String(S.findMatches('abc', 'x*', { regex: true }).length > 0))

		// An unusable pattern is reported, not silently answered with "no
		// matches" — which is indistinguishable from a document that lacks it.
		let threw = false
		try { S.findMatches('abc', '(unclosed', { regex: true }) } catch { threw = true }
		out.push(String(threw))

		return out
	})()`, &got)

	want := []string{
		"0-3,8-11",
		"2",
		"0-3,4-7,8-11",
		"4-7",
		"0-3",
		"5-10",
		"0-2,3-5,6-8",
		"0-3",
		"0",
		"true",
		"true",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDocumentReplaceRewritesTheSource(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const S = await import('/src/markdown/search.js')
		const out = []

		// One match at a time.
		const m = S.findMatches('one two one', 'one')
		out.push(S.replaceMatch('one two one', m[1], 'X'))

		// Replace-all with a LONGER replacement is where a forward loop
		// corrupts the document: every later match shifts by the difference in
		// length, so the second replacement lands in the wrong place.
		out.push(S.replaceAllMatches('one two one', 'one', 'ONE-LONGER').text)
		out.push(String(S.replaceAllMatches('one two one', 'one', 'X').count))

		// A SHORTER replacement shifts the other way.
		out.push(S.replaceAllMatches('aaa bbb aaa', 'aaa', 'z').text)

		// The replacement is literal, in regex mode too: a $ in replacement text
		// is a dollar sign, not a capture reference.
		out.push(S.replaceAllMatches('price here', 'price', '$1 cost').text)

		// Replacing with the empty string is a deletion, not a no-op.
		out.push(S.replaceAllMatches('keep drop keep', 'drop ', '').text)

		// No matches must leave the document byte-identical.
		out.push(String(S.replaceAllMatches('untouched', 'absent', 'X').text === 'untouched'))

		return out
	})()`, &got)

	want := []string{
		"one two X",
		"ONE-LONGER two ONE-LONGER",
		"2",
		"z bbb z",
		"$1 cost here",
		"keep keep",
		"true",
	}
	for i := range want {
		if i >= len(got) {
			t.Fatalf("got only %d results, want %d", len(got), len(want))
		}
		if got[i] != want[i] {
			t.Errorf("result %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// The formatted surface has no source offsets, so a match is revealed there by
// the block it falls in. The mapping is derived from the same source string as
// the match, which is what stops it disagreeing with the match count — a second
// search over the rendered DOM would have counted different things, since
// markdown syntax characters are not in the rendered text.
func TestSourceOffsetsMapToBlocksForTheFormattedSurface(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const S = await import('/src/markdown/search.js')
		const doc = '# Title\n\nfirst para\n\nsecond para\n\n- a list item\n'
		const at = (needle) => String(S.sourceBlockIndex(doc, doc.indexOf(needle)))
		return [at('Title'), at('first'), at('second'), at('list item')]
	})()`, &got)

	want := []string{"0", "1", "2", "3"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Errorf("block index %d = %q, want %q (got %q)", i, got[min(i, len(got)-1)], want[i], got)
		}
	}
}
