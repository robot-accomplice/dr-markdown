package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// jsString renders a Go string as a JavaScript string literal, so a fixture
// with newlines can be embedded in an evaluated expression without
// hand-escaping.
func jsString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// probeCrepeJS creates a throwaway Crepe on a detached div and returns it.
//
// This is the method the 2026-08-09 source-positions probe used, and it is
// chosen so the probe touches nothing the app runs: no import of editor.js, no
// fidelity registry, no __app surface. The instance exists only to hand over
// `remark`, which the bundle does not export but the live ctx does.
const probeCrepeJS = `
  async function newProbeCrepe(markdown) {
    const { Crepe } = await import('/vendor/crepe.bundle.mjs')
    const host = document.createElement('div')
    document.body.appendChild(host)
    const crepe = new Crepe({ root: host, defaultValue: markdown })
    await crepe.create()
    return { crepe, host }
  }
`

// The approach diffs SYNTAX TREES rather than texts, and that only closes the
// re-serialization class if the respelling lives in the serializer rather than
// in the tree. Two claims carry the whole design:
//
//  1. a table's mdast is invariant under delimiter-row padding
//  2. character references are decoded at parse time
//
// Both were predicted, neither measured. This project has three times recorded
// a conclusion with true premises that was false anyway, so they are measured
// here BEFORE any diff code exists. If this test fails, the approach is dead.
func TestRespellingLivesInTheSerializerNotTheTree(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got struct {
		TableEqual  bool   `json:"tableEqual"`
		EntityValue string `json:"entityValue"`
		TablePadded string `json:"tablePadded"`
		TableTerse  string `json:"tableTerse"`
	}
	evalJS(t, ctx, probeCrepeJS+`(async () => {
	  const { crepe, host } = await newProbeCrepe('x')
	  const remark = crepe.editor.ctx.get('remark')

	  const strip = (node) => JSON.stringify(node, (k, v) => (k === 'position' ? undefined : v))
	  const padded = remark.parse('| a | b |\n| --- | --- |\n| 1 | 2 |\n')
	  const terse  = remark.parse('| a | b |\n| - | - |\n| 1 | 2 |\n')
	  const entity = remark.parse('&amp;\n')

	  const result = {
	    tableEqual: strip(padded) === strip(terse),
	    entityValue: entity.children[0].children[0].value,
	    tablePadded: strip(padded),
	    tableTerse: strip(terse),
	  }
	  await crepe.destroy().catch(() => {})
	  host.remove()
	  return result
	})()`, &got)

	if !got.TableEqual {
		t.Errorf("PREDICTION 1 REFUTED: delimiter-row padding changes the mdast, so a structural "+
			"diff cannot preserve it. The approach is dead; record this and stop.\npadded: %s\nterse:  %s",
			got.TablePadded, got.TableTerse)
	}
	if got.EntityValue != "&" {
		t.Errorf("PREDICTION 2 REFUTED: `&amp;` parses to %q, not \"&\", so entity decoding is not "+
			"a parse-time normalization and the tree is not respelling-invariant", got.EntityValue)
	}
}

// structuralEquals is the whole design in one function: it must call two nodes
// equal when they differ ONLY in where they came from, and unequal on any real
// content difference. Driven as a pure module, with no editor at all.
func TestStructuralEqualsIgnoresPositionAndNothingElse(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
	  const S = await import('/src/markdown/sourcepatch.js')
	  const at = (o) => ({ start: { offset: o }, end: { offset: o + 1 } })
	  return [
	    String(S.structuralEquals(
	      { type: 'text', value: 'a', position: at(0) },
	      { type: 'text', value: 'a', position: at(99) })),
	    String(S.structuralEquals(
	      { type: 'text', value: 'a' },
	      { type: 'text', value: 'b' })),
	    String(S.structuralEquals(
	      { type: 'heading', depth: 1, children: [{ type: 'text', value: 'x', position: at(2) }] },
	      { type: 'heading', depth: 1, children: [{ type: 'text', value: 'x', position: at(7) }] })),
	    String(S.structuralEquals(
	      { type: 'heading', depth: 1, children: [] },
	      { type: 'heading', depth: 2, children: [] })),
	    String(S.structuralEquals({ type: 'a', children: [] }, { type: 'a' })),
	    String(S.structuralEquals(null, null)),
	    String(S.structuralEquals({ type: 'x' }, null)),
	  ]
	})()`, &got)

	want := []string{"true", "false", "true", "false", "false", "true", "false"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

// A naive positional comparison would mark every block after an edit as
// changed, and re-splicing an untouched table is exactly the defect this
// design exists to avoid. LCS is what keeps the blast radius at one block.
func TestAlignMatchesUnchangedBlocksAcrossAnEdit(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
	  const S = await import('/src/markdown/sourcepatch.js')
	  const n = (v) => ({ type: 'paragraph', value: v })
	  const shape = (ops) => ops.map((o) =>
	    o.type === 'match' ? 'M' + o.a.value : 'C[' +
	      o.as.map((x) => x.value).join('') + '/' + o.bs.map((x) => x.value).join('') + ']').join(' ')
	  return [
	    shape(S.align([n('a'), n('b'), n('c')], [n('a'), n('B'), n('c')])),
	    shape(S.align([n('a'), n('b')], [n('a'), n('b')])),
	    shape(S.align([n('a'), n('b')], [n('a')])),
	    shape(S.align([n('a')], [n('a'), n('b')])),
	    shape(S.align([], [])),
	  ]
	})()`, &got)

	want := []string{
		"Ma C[b/B] Mc", // the edit is isolated; `a` and `c` still match
		"Ma Mb",        // nothing changed
		"Ma C[b/]",     // deletion: `as` populated, `bs` empty
		"Ma C[/b]",     // insertion: `as` empty, `bs` populated
		"",             // degenerate
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// The fixture is deliberately isolated: no frontmatter and no hard break, so
// neither the frontmatter split nor the break-sentinel substitution fires and
// offsets into the body equal offsets into the file. A failure here therefore
// means the diff is wrong, not the translation.
const patchFixture = "Intro paragraph with one word to change.\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n"

// With NO edit, every node is structurally equal, so nothing is spliced and the
// original must come back untouched. That is the whole premise in one
// assertion, and it needs no keystroke, no focus handling and no async
// settling.
//
// It is meaningful precisely because the editor's own serialization of this
// fixture is NOT byte-identical to it: the delimiter row comes back as
// `| - | - |`. The test asserts that too, so a future editor that stopped
// respelling would make this test explain itself rather than pass vacuously.
func TestPatchWithNoEditReturnsTheOriginalBytes(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got struct {
		Serialized string `json:"serialized"`
		Patched    string `json:"patched"`
	}
	evalJS(t, ctx, probeCrepeJS+`(async () => {
	  const S = await import('/src/markdown/sourcepatch.js')
	  const original = `+jsString(patchFixture)+`
	  const { crepe, host } = await newProbeCrepe(original)
	  const remark = crepe.editor.ctx.get('remark')
	  const serialized = crepe.getMarkdown()
	  const patched = S.patchPreservingSource(original, serialized, remark)
	  await crepe.destroy().catch(() => {})
	  host.remove()
	  return { serialized, patched }
	})()`, &got)

	if got.Serialized == patchFixture {
		t.Fatal("the editor no longer respells this fixture, so this test proves nothing. " +
			"Pick a fixture the editor still rewrites, or retire the probe.")
	}
	if got.Patched != patchFixture {
		t.Errorf("patch did not return the original bytes.\n got: %q\nwant: %q\n(editor produced %q)",
			got.Patched, patchFixture, got.Serialized)
	}
}

// The gate the probe exists to pass: one word changes, and the table's
// delimiter row is still `| --- | --- |` because nothing ever touched those
// bytes.
//
// The edit is made through the DOM selection rather than by dispatching a
// ProseMirror transaction. That keeps the probe honest — the transaction API is
// the mechanism this design set out to avoid depending on, so using it to
// manufacture the input would beg the question.
func TestOneWordEditLeavesEveryOtherByteAlone(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	const want = "Intro paragraph with one WORD to change.\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n"

	var got struct {
		Edited     bool   `json:"edited"`
		Serialized string `json:"serialized"`
		Patched    string `json:"patched"`
	}
	evalJS(t, ctx, probeCrepeJS+`(async () => {
	  const S = await import('/src/markdown/sourcepatch.js')
	  const original = `+jsString(patchFixture)+`
	  const { crepe, host } = await newProbeCrepe(original)
	  const remark = crepe.editor.ctx.get('remark')

	  // Find the text node holding "word" by walking, rather than by guessing a
	  // class name off the vendored bundle's internals.
	  const walker = document.createTreeWalker(host, NodeFilter.SHOW_TEXT)
	  let node = null
	  while (walker.nextNode()) {
	    if (walker.currentNode.nodeValue.includes('word')) { node = walker.currentNode; break }
	  }
	  if (!node) {
	    await crepe.destroy().catch(() => {})
	    host.remove()
	    return { edited: false, serialized: '', patched: '' }
	  }

	  const editable = host.querySelector('[contenteditable="true"]')
	  editable.focus()
	  const at = node.nodeValue.indexOf('word')
	  const range = document.createRange()
	  range.setStart(node, at)
	  range.setEnd(node, at + 'word'.length)
	  const selection = window.getSelection()
	  selection.removeAllRanges()
	  selection.addRange(range)
	  document.execCommand('insertText', false, 'WORD')

	  await new Promise((resolve) => setTimeout(resolve, 250))
	  const serialized = crepe.getMarkdown()
	  const patched = S.patchPreservingSource(original, serialized, remark)
	  await crepe.destroy().catch(() => {})
	  host.remove()
	  return { edited: true, serialized, patched }
	})()`, &got)

	// A test that never made an edit must not be read as a result either way.
	if !got.Edited {
		t.Fatal("the harness never found the word to edit, so nothing was measured")
	}
	if !strings.Contains(got.Serialized, "WORD") {
		t.Fatalf("the editor did not observe the edit, so nothing was measured: %q", got.Serialized)
	}

	if got.Patched != want {
		t.Errorf("the edit did not stay inside the paragraph.\n got: %q\nwant: %q\n(editor produced %q)",
			got.Patched, want, got.Serialized)
	}
	if !strings.Contains(got.Patched, "| --- | --- |") {
		t.Error("the delimiter row was respelled, which is the exact defect this probe exists to close")
	}
}
