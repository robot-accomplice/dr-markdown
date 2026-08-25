//go:build darwin

package main

// A realistic document, run through the real editor in the real host.
//
// The round-trip gate used one paragraph and one table, which is a document
// with no structure. Real files interleave constructs, and the interesting
// failures live BETWEEN elements rather than inside them: whether two fenced
// blocks back to back stay two blocks, whether a blank line between them
// survives, whether a multi-line fence keeps its interior newlines and
// indentation, whether a list that follows a fence is still a list.
//
// Reported as a diff rather than asserted byte-identical. Some rewriting is
// known and tracked (the re-serialization blocker); the point here is to see
// exactly WHAT a structured document loses, not to re-fail a known bug.
// docFixturePath is the file under test, given with -doc <path>.
var docFixturePath string

const compositeDocJS = "" +
	// Fetched rather than embedded: the fixture is a real checked-in document,
	// so it can be edited and re-run without rebuilding, and what is validated
	// is the same file the corpus gate reads.
	"const res = await fetch('drmd://app/__docfixture.md')\n" +
	"const fixture = await res.text()\n" +
	"for (let i = 0; i < 200 && !globalThis.__app?.ready; i++) {\n" +
	"  await new Promise((r) => setTimeout(r, 50))\n" +
	"}\n" +
	"await globalThis.__app.setMarkdown(fixture)\n" +
	"await new Promise((r) => setTimeout(r, 500))\n" +
	"const out = globalThis.__app.getEditorMarkdown()\n" +
	// Second pass: a fixture must be STABLE even when it is not byte-identical.
	"await globalThis.__app.setMarkdown(out)\n" +
	"await new Promise((r) => setTimeout(r, 500))\n" +
	"const again = globalThis.__app.getEditorMarkdown()\n" +
	"window.webkit.messageHandlers.drmd.postMessage({\n" +
	"  id: 0, method: '__doc', args: [{ input: fixture, output: out, second: again }],\n" +
	"})\n"
