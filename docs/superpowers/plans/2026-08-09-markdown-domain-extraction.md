# Markdown Domain Extraction Implementation Plan (Phase 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the 360 lines of pure markdown logic out of `app.js` into six domain modules, so the product's document rules stop living inside its DOM adapter and become directly verifiable.

**Architecture:** Six modules under `frontend/dist/src/markdown/`. Five are leaves with **zero** dependencies on anything else in `app.js` — measured, not assumed. The sixth, `commands.js`, imports two of them. `app.js` keeps every DOM handler, every `state` mutation and every `bridge` call, and imports the domain instead of containing it.

**Tech Stack:** Plain ES modules (no bundler, no Node toolchain). Go 1.26.5 + chromedp for all frontend testing.

## Global Constraints

- **No Node toolchain.** `node --check` in CI is a syntax gate only. Pure modules are exercised by importing them into an already-served page through chromedp, exactly as `e2e/fidelity_unit_test.go` does.
- **Behaviour-preserving.** This phase changes structure, not behaviour.
- **The judging criterion:** every existing test must stay green **and unchanged** — `TestRoundTripCorpus`, `TestFidelitySurveyRewritesExactlyTheseConstructs`, `TestWysiwygRewritesTheseConstructs`, `TestOpeningADocumentDoesNotMarkItModified`, `TestFailureSurfacesRecordDurableEvents`, `TestObfuscatedSchemesAreRefusedInRenderedLinks`, and everything in `e2e/e2e_test.go`. A task that needs any of them edited to pass has changed behaviour and is wrong — stop and report rather than editing the test.
- **Carry the comments across verbatim.** A moved function moves its comment block unchanged.
- **Every commit runs the full gate:** `gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1`. CI enforces gofmt as its first step, so a gate that omits it is a subset of the real one — that is how a green local run reached a red CI on this very branch.
- Attribution rule: no Claude/Anthropic co-author trailers or footers in commits or PRs.

## What was measured before planning

Every top-level function in `app.js` was classified by whether its body touches `document.`, `els.`, `state.`, `bridge.`, `activeDoc()` or `window.`, and then its call graph was checked against the proposed module boundaries.

| module | functions | lines | constants it needs | calls outside its own group |
| --- | --- | --- | --- | --- |
| `tables.js` | 14 | 85 | — | **none** |
| `fences.js` | 6 | 76 | — | **none** |
| `images.js` | 6 | 44 | `IMAGE_TOKEN_SOURCE` | **none** |
| `links.js` | 2 | 12 | `SAFE_LINK_SCHEMES` | **none** |
| `text.js` | 4 | 13 | `CRLF` | **none** |
| `commands.js` | 13 | 130 | — | `tables`, `images` (imports them) |

**360 lines, and five of the six modules are leaves.** That is why this phase is a sequence of small green steps rather than a rewrite: each leaf can be moved and verified on its own.

`applyBlockStyle` is deliberately **not** in the list. It is `function applyBlockStyle(style) { runCommand(style) }` — a three-line pass-through to the shell dispatcher, with no logic. It is shell glue, not domain code, and stays in `app.js`. It was the only back-edge from the candidate set into the impure side, and this is why it is not a real one.

## Defects found while planning, fixed in the task that touches their file

- **`titleForPath` has a dead conditional.** `return fallback === 'doc-1' ? 'Untitled.md' : 'Untitled.md'` — both branches are the same string, so the condition never matters and the `fallback` parameter does nothing. Fixed in Task 5.
- **`startEditing(started = true)`** (Phase 1 audit F3) takes a boolean that is never `false`, with nine call sites passing `true` redundantly. This phase opens `app.js`, so it is fixed here, in Task 7.

## File Structure

**Create:**

| file | responsibility |
| --- | --- |
| `frontend/dist/src/markdown/tables.js` | GFM table geometry: find a table, add/remove rows and columns, set alignment, generate one. |
| `frontend/dist/src/markdown/fences.js` | Fenced code blocks: read the first fence's language, rewrite a fence's language or its mermaid source. |
| `frontend/dist/src/markdown/images.js` | Image tokens in both markdown and `<img>` form: locate, parse, format. |
| `frontend/dist/src/markdown/links.js` | Link href safety: normalize a href the way the URL parser will, then allow only navigable schemes. |
| `frontend/dist/src/markdown/text.js` | Document text conventions: line endings and the title shown for a path. |
| `frontend/dist/src/markdown/commands.js` | Editor commands as document transforms: given a command, the document and the selection, return the new document. |
| `e2e/markdown_unit_test.go` | Direct in-page exercise of every module. One browser boot per module, table-driven. |

**Modify:** `frontend/dist/src/app.js` (delete the moved functions, import them instead), `docs/architext/data/nodes.json`, `docs/architext/data/decisions.json`.

## Deferred from this phase, with reasons

- **`wire()` (124 lines)** is a flat DOM event-binding table. It is view glue, not domain logic, so it is outside this phase's stated boundary. Splitting it is a view-layer concern and belongs with any future work on the shell.
- **`app.js`'s panel, settings and preview rendering** stays. The design says explicitly that this is genuinely view code and belongs in the outer circle; only the pure logic embedded alongside it moves.
- **Phase 3 (Go use cases)** gets its own plan. Dependency inversion already exists there, so it addresses comprehension rather than correctness.

---

### Task 1: Extract `tables.js`

**Files:**
- Create: `frontend/dist/src/markdown/tables.js`
- Modify: `frontend/dist/src/app.js`
- Test: `e2e/markdown_unit_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces, all exported from `markdown/tables.js`:
  - `tableMarkdown(cols: number, rows: number) -> string`
  - `tableRow(cells: string[]) -> string`
  - `splitTableRow(row: string) -> string[]`
  - `isTableRow(line: string) -> boolean`
  - `isDividerRow(line: string) -> boolean`
  - `tableBounds(md: string, tableIndex?: number) -> { lines: string[], start: number, end: number } | null`
  - `containsTable(md: string) -> boolean`
  - `rewriteTable(md: string, tableIndex: number, transform: (rows: string[]) => string[]) -> string`
  - `addTableRow(md, tableIndex?) -> string`, `removeTableRow(md, tableIndex?) -> string`
  - `addTableColumn(md, tableIndex?) -> string`, `removeTableColumn(md, tableIndex?) -> string`
  - `alignTable(md, alignment: 'left'|'center'|'right', tableIndex?) -> string`
  - `deleteTable(md, tableIndex?) -> string`

- [ ] **Step 1: Write the failing test**

Create `e2e/markdown_unit_test.go`:

```go
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
```

The last case matters: every table operation must be a no-op on a document with no table, because the command dispatcher can reach them with the cursor anywhere.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestTableOperations -count=1 -v
```

Expected: FAIL with `Failed to fetch dynamically imported module: .../src/markdown/tables.js`.

- [ ] **Step 3: Create the module**

Create `frontend/dist/src/markdown/tables.js` containing `tableMarkdown`, `tableRow`, `splitTableRow`, `isTableRow`, `isDividerRow`, `tableBounds`, `containsTable`, `rewriteTable`, `addTableRow`, `removeTableRow`, `addTableColumn`, `removeTableColumn`, `alignTable` and `deleteTable`, moved **verbatim** from `app.js` with any comment blocks attached, each prefixed with `export`. Add a module header:

```js
// GFM table geometry: locating a table in a document and rewriting its shape.
//
// Every operation is a no-op when the document has no table at the requested
// index, because the command dispatcher can reach these with the cursor
// anywhere — `tableBounds` returning null is the normal case, not an error.
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestTableOperations -count=1 -v
```

Expected: PASS. If any `want` above disagrees with the moved code, **the want is wrong, not the code** — this is a behaviour-preserving move, so correct the test to what the existing implementation produces and note it in the commit.

- [ ] **Step 5: Rewire `app.js`**

Delete the fourteen functions from `app.js`. Add at the top of the import block:

```js
import {
  tableMarkdown, tableRow, splitTableRow, isTableRow, isDividerRow, tableBounds,
  containsTable, rewriteTable, addTableRow, removeTableRow, addTableColumn,
  removeTableColumn, alignTable, deleteTable,
} from './markdown/tables.js'
```

Then remove any name from that import list that `app.js` no longer references, so the import surface matches actual use rather than the module's full API.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: all green, no existing test edited. `TestContextualDocumentControlsManageBlocksInPlace` in `e2e/e2e_test.go` is the sharp check — it drives the table controls through the real UI.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/markdown/tables.js frontend/dist/src/app.js e2e/markdown_unit_test.go
git commit -m "refactor: extract table operations into markdown/tables.js

Fourteen functions, 85 lines, zero dependencies on anything else in app.js —
measured from the call graph before the move, not assumed.

Directly tested now, including the case the UI made awkward to reach: every
table operation is a no-op on a document with no table, which matters because
the command dispatcher can reach them with the cursor anywhere."
```

---

### Task 2: Extract `fences.js`

**Files:**
- Create: `frontend/dist/src/markdown/fences.js`
- Modify: `frontend/dist/src/app.js`
- Test: `e2e/markdown_unit_test.go`

**Interfaces:**
- Consumes: `normalizeLanguage` from `../highlighter.js`.
- Produces:
  - `firstCodeFenceDescriptor(md, opts?: { excludeMermaid?: boolean, onlyMermaid?: boolean }) -> { language: string, ... } | null`
  - `firstCodeFenceLanguage(md, opts?) -> string`
  - `rewriteCodeFenceLanguage(md, fenceIndex: number, language: string) -> string` — a non-integer `fenceIndex` is a no-op
  - `containsMermaidDiagram(md) -> boolean`
  - `rewriteMermaidFenceSource(md, source) -> string`
  - `fencedLanguages(md) -> string[]`

- [ ] **Step 1: Write the failing test**

Append to `e2e/markdown_unit_test.go`:

```go
func TestFenceOperations(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const F = await import('/src/markdown/fences.js')
		const js = '```js\nconst a = 1\n```\n'
		const mermaid = '```mermaid\ngraph TD\n```\n'
		return [
			F.firstCodeFenceLanguage(js),
			F.firstCodeFenceLanguage('no fence here'),
			String(F.containsMermaidDiagram(mermaid)),
			String(F.containsMermaidDiagram(js)),
			F.fencedLanguages(js + mermaid).join(','),
			F.rewriteCodeFenceLanguage(js, 'python'),
		]
	})()`, &got)

	want := []string{
		"js",
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
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestFenceOperations -count=1 -v
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create the module**

Move `firstCodeFenceLanguage`, `firstCodeFenceDescriptor`, `rewriteCodeFenceLanguage`, `containsMermaidDiagram`, `rewriteMermaidFenceSource` and `fencedLanguages` verbatim into `frontend/dist/src/markdown/fences.js`, each prefixed with `export`, comments attached. Module header:

```js
// Fenced code blocks: reading a fence's info string and rewriting a fence's
// language or its body. Mermaid is a fenced block with a known language rather
// than a separate construct, so it is handled here rather than in its own
// module — `onlyMermaid` and `excludeMermaid` exist so callers can ask for the
// diagram or for the code, from the same scan.
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestFenceOperations -count=1 -v
```

Expected: PASS. If a `want` disagrees, correct the test to the existing behaviour and note it — this is a move.

- [ ] **Step 5: Rewire `app.js`**

Delete the six functions; import the ones `app.js` still uses from `./markdown/fences.js`.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. The mermaid and code-assistant e2e tests are the sharp checks.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/markdown/fences.js frontend/dist/src/app.js e2e/markdown_unit_test.go
git commit -m "refactor: extract fenced-code operations into markdown/fences.js

Six functions, 76 lines, no dependencies on the rest of app.js. Mermaid lives
here rather than in its own module because it is a fenced block with a known
language, not a separate construct."
```

---

### Task 3: Extract `images.js`

**Files:**
- Create: `frontend/dist/src/markdown/images.js`
- Modify: `frontend/dist/src/app.js`
- Test: `e2e/markdown_unit_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `IMAGE_TOKEN_SOURCE: string` — the regex source, exported because `app.js` builds its own `RegExp` from it.
  - `imageTokens(md) -> { text: string, start: number, end: number }[]`
  - `parseImageToken(text) -> { alt: string, path: string, width: string }`
  - `formatImageToken({ alt, path, width }) -> string`
  - `selectedImageToken(md, index) -> token | undefined`
  - `rewriteImage(md, index, transform) -> string`
  - `htmlImageAttribute(tag, name) -> string`

- [ ] **Step 1: Write the failing test**

```go
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
```

Both token forms are covered because image sizing is written as an `<img>` tag — CommonMark has no size syntax — so the two forms are not interchangeable and a parser that handles only one silently drops sizing.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestImageTokens -count=1 -v
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create the module**

Move the six functions and the `IMAGE_TOKEN_SOURCE` constant verbatim into `frontend/dist/src/markdown/images.js`, exporting each. Leave `IMAGE_WIDTH_PRESETS` in `app.js` — it is a list of UI preset buttons, not document logic. Module header:

```js
// Image tokens in a markdown document, in both forms the product writes.
//
// Sizing is expressed as an `<img src alt width>` tag because CommonMark has no
// size syntax, so a document can hold either form and they are not
// interchangeable — a parser that handles only `![alt](path)` silently drops
// every sized image's width.
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestImageTokens -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Rewire `app.js`**

Delete the six functions and the constant; import what is still referenced from `./markdown/images.js`.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. `TestImageWidthRoundTripsBetweenMarkdownAndHTMLForms` is the sharp check.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/markdown/images.js frontend/dist/src/app.js e2e/markdown_unit_test.go
git commit -m "refactor: extract image-token parsing into markdown/images.js

Six functions plus IMAGE_TOKEN_SOURCE, 44 lines, no dependencies on the rest of
app.js. IMAGE_WIDTH_PRESETS stays behind — it is a list of UI buttons, not
document logic.

Both token forms are directly tested. Sizing is written as an <img> tag because
CommonMark has no size syntax, so a parser handling only ![alt](path) silently
drops every sized image's width."
```

---

### Task 4: Extract `links.js`

**Files:**
- Create: `frontend/dist/src/markdown/links.js`
- Modify: `frontend/dist/src/app.js`
- Test: `e2e/markdown_unit_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `SAFE_LINK_SCHEMES: string[]`
  - `normalizeHref(href) -> string`
  - `safeLinkHref(href) -> string | null` — the href to assign, or `null` to refuse.

`recordRefusedLink` stays in `app.js`: it calls `bridge.recordEvent` and owns the dedupe cap, so it is adapter code, not domain.

- [ ] **Step 1: Write the failing test**

```go
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
		"https://example.com/page",
		"mailto:a@b.c",
		"notes/other.md",
		"null", "null", "null", "null", "null", "null", "null",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}
```

These are the obfuscation cases from `TestObfuscatedSchemesAreRefusedInRenderedLinks`, asserted against the function directly. The rendered-link test stays exactly as it is — it proves the check is *wired in*, which this one cannot.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestLinkSafety -count=1 -v
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create the module**

Move `normalizeHref`, `safeLinkHref` and `SAFE_LINK_SCHEMES` verbatim into `frontend/dist/src/markdown/links.js`, exporting each, with the existing comment about returning an href rather than a boolean. Module header:

```js
// Link href safety.
//
// The check must be run on the string the BROWSER will navigate to, not on the
// string as written: the URL parser strips ASCII tab, LF and CR from anywhere
// in a URL before parsing, so `jav<TAB>ascript:` has no scheme by a regex's
// reading and `javascript:` by the parser's. normalizeHref does that stripping
// first so both readings agree.
//
// This matters more here than in a browser tab: a javascript: URL runs in the
// app's own origin, where the Wails bindings expose SaveDocument and
// OpenRecentDocument with no path restriction. This product exists to open
// ARBITRARY markdown, so every document is untrusted input.
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestLinkSafety -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Rewire `app.js`**

Delete the two functions and the constant; import them from `./markdown/links.js`. `recordRefusedLink` and `handleDocumentLinkClick` stay.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. `TestObfuscatedSchemesAreRefusedInRenderedLinks` and `TestFailureSurfacesRecordDurableEvents/refused_link_scheme` are the sharp checks — the second proves the refusal still reaches the audit trail.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/markdown/links.js frontend/dist/src/app.js e2e/markdown_unit_test.go
git commit -m "refactor: extract link-scheme safety into markdown/links.js

Two functions and the scheme allowlist. recordRefusedLink stays in app.js: it
calls bridge.recordEvent and owns the flood cap, so it is adapter code.

The ten obfuscation cases are asserted against the function directly now. The
rendered-link test is unchanged and still required — it proves the check is
WIRED IN, which a direct unit test cannot."
```

---

### Task 5: Extract `text.js`, and fix a dead conditional

**Files:**
- Create: `frontend/dist/src/markdown/text.js`
- Modify: `frontend/dist/src/app.js`
- Test: `e2e/markdown_unit_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `CRLF: string`
  - `detectLineEnding(text) -> string`
  - `toEditorText(text) -> string`
  - `toFileText(text, lineEnding) -> string`
  - `titleForPath(path) -> string` — **note the changed arity**, see below.

`titleForPath` currently reads:

```js
function titleForPath(path, fallback) {
  if (!path) return fallback === 'doc-1' ? 'Untitled.md' : 'Untitled.md'
  return path.split(/[\\/]/).pop() || path
}
```

Both branches of the ternary return the same string, so the condition is dead and `fallback` is unused. The move drops both. This is the one behaviour-adjacent change in the phase and it is provably none: the function returns `'Untitled.md'` for a falsy path either way.

- [ ] **Step 1: Write the failing test**

```go
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
```

The Windows path case matters: the split is on `[\\/]` precisely so a Windows-authored path does not show its whole directory chain as the tab title.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestDocumentTextConventions -count=1 -v
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create the module**

Move `CRLF`, `detectLineEnding`, `toEditorText`, `toFileText` and `titleForPath` into `frontend/dist/src/markdown/text.js`, exporting each. Write `titleForPath` as:

```js
// The name shown for a document, from its path. Split on both separators so a
// Windows-authored path does not show its whole directory chain as the title.
//
// The unused `fallback` parameter and its dead ternary were dropped in the
// move: both branches returned 'Untitled.md', so the condition never mattered.
export function titleForPath(path) {
  if (!path) return 'Untitled.md'
  return path.split(/[\\/]/).pop() || path
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestDocumentTextConventions -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Rewire `app.js`**

Delete the four functions and the constant; import from `./markdown/text.js`. Then find every `titleForPath(` call site and drop the second argument:

```bash
grep -n "titleForPath(" frontend/dist/src/app.js
```

Every call must now pass exactly one argument.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. `TestLineEndingsSurviveAnEditAndSave` is the sharp check for the CRLF functions, and the tab-title assertions in `e2e/e2e_test.go` for `titleForPath`.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/markdown/text.js frontend/dist/src/app.js e2e/markdown_unit_test.go
git commit -m "refactor: extract line-ending and title conventions into markdown/text.js

Also drops a dead conditional found while planning: titleForPath returned
'Untitled.md' from BOTH branches of a ternary on its fallback parameter, so the
condition never mattered and the parameter did nothing. Both are gone and every
call site now passes one argument.

Provably behaviour-preserving: the function returned 'Untitled.md' for a falsy
path either way."
```

---

### Task 6: Extract `commands.js`

**Files:**
- Create: `frontend/dist/src/markdown/commands.js`
- Modify: `frontend/dist/src/app.js`
- Test: `e2e/markdown_unit_test.go`

**Interfaces:**
- Consumes: `tables.js` (`tableMarkdown`, `addTableRow`, `removeTableRow`, `addTableColumn`, `removeTableColumn`, `alignTable`, `deleteTable`), `images.js` (`rewriteImage`).
- Produces:
  - `applyCommand(command: string, md: string, editorContext?: { selectionText?: string, blockText?: string }) -> string`
  - plus `appendBlock`, `replaceFirstSelection`, `rewriteLastMatchingLine`, `indentLastListItem`, `outdentLastListItem`, and the four `*ContainingSelection` helpers, exported for `app.js`'s existing callers.

This is the only module that imports another, and it is why it goes last.

- [ ] **Step 1: Write the failing test**

```go
func TestCommandTransforms(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const C = await import('/src/markdown/commands.js')
		const sel = (cmd, md, selection) => C.applyCommand(cmd, md, { selectionText: selection })
		const blk = (cmd, md, block) => C.applyCommand(cmd, md, { blockText: block })
		return [
			sel('bold', 'make this bold\n', 'this'),
			sel('italic', 'make this italic\n', 'this'),
			blk('h1', 'a heading\n', 'a heading'),
			blk('quote', 'a line\n', 'a line'),
			blk('bullet-list', 'an item\n', 'an item'),
			C.appendBlock('# T', 'new block'),
			C.appendBlock('', 'first block'),
			C.replaceFirstSelection('aXbXc', 'X', (s) => s.toUpperCase()),
			C.rewriteLastMatchingLine('- a\n- b\n', /^- /, (l) => l + '!'),
		]
	})()`, &got)

	want := []string{
		"make **this** bold\n",
		"make *this* italic\n",
		"# a heading\n",
		"> a line\n",
		"- an item\n",
		"# T\n\nnew block\n",
		"first block\n",
		"aXbXc",
		"- a\n- b!\n",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}
```

If any `want` disagrees with the moved implementation, correct the test — this is a move, and the existing behaviour is the specification. Note each correction in the commit message, because a `want` that had to change is a place where the behaviour was not what a reader would predict, and that is worth recording.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestCommandTransforms -count=1 -v
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create the module**

Move `applyCommand`, `applySelectionCommand`, `applyCurrentBlockCommand`, `replaceFirstSelection`, `formatLineContainingSelection`, `quoteLineContainingSelection`, `listLineContainingSelection`, `codeBlockContainingSelection`, `rewriteLineContainingSelection`, `appendBlock`, `rewriteLastMatchingLine`, `indentLastListItem` and `outdentLastListItem` verbatim into `frontend/dist/src/markdown/commands.js`, exporting each. Add at the top:

```js
// Editor commands as document transforms: given a command name, the document
// and what the user has selected, return the new document. No DOM, no editor
// instance — which is what makes every command directly testable.
import {
  tableMarkdown, addTableRow, removeTableRow, addTableColumn, removeTableColumn,
  alignTable, deleteTable,
} from './tables.js'
import { rewriteImage } from './images.js'
```

Trim that import list to the names `applyCommand` actually calls.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestCommandTransforms -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Rewire `app.js`**

Delete the thirteen functions; import from `./markdown/commands.js`. `runCommand` and `applyBlockStyle` stay — they dispatch, touch `state` and call `bridge`.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. The formatting-command e2e tests and `TestKeyboardShortcutRunsCommand` are the sharp checks.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/markdown/commands.js frontend/dist/src/app.js e2e/markdown_unit_test.go
git commit -m "refactor: extract command transforms into markdown/commands.js

Thirteen functions, 130 lines. The only module that imports another — tables and
images — which is why it went last.

runCommand and applyBlockStyle stay in app.js: they dispatch, touch state and
call bridge, so they are adapter code. applyCommand is the domain entry point:
command plus document plus selection in, new document out."
```

---

### Task 7: Remove the never-false flag, and record the phase

**Files:**
- Modify: `frontend/dist/src/app.js`
- Modify: `docs/architext/data/nodes.json`, `docs/architext/data/decisions.json`

**Interfaces:**
- Consumes: nothing.
- Produces: `startEditing()` with no parameters.

- [ ] **Step 1: Write the failing test**

Append to `e2e/markdown_unit_test.go`:

```go
// Phase 1's clean-code audit found startEditing taking a boolean that is never
// false, defaulting to true, with nine call sites passing true redundantly.
// Dead configurability reads as a decision the caller gets to make.
func TestStartEditingTakesNoFlag(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "frontend", "dist", "src", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "startEditing(true)") {
		t.Error("startEditing is still called with a redundant true argument")
	}
	if strings.Contains(string(source), "function startEditing(started") {
		t.Error("startEditing still takes a parameter that is never false")
	}
}
```

Add `"os"`, `"path/filepath"` and `"strings"` to the file's imports.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestStartEditingTakesNoFlag -count=1 -v
```

Expected: FAIL on both assertions.

- [ ] **Step 3: Remove the flag**

```js
function startEditing() {
  const doc = activeDoc()
  if (doc) doc.started = true
}
```

Then replace every `startEditing(true)` with `startEditing()`:

```bash
grep -c "startEditing(true)" frontend/dist/src/app.js   # expect 9
```

Note the simplification this exposes: the old body was `doc.started = started || doc.markdown.length > 0`. With `started` always `true`, the `||` is dead too.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestStartEditingTakesNoFlag -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green.

- [ ] **Step 6: Record the phase in Architext**

In `docs/architext/data/nodes.json`, add `frontend/dist/src/markdown/` modules to the `frontend-shell` node's `sourcePaths`, and add a responsibility:

```
"Delegate document transforms to the markdown domain modules rather than implementing them: tables, fenced code, image tokens, link safety, line endings and commands"
```

In `docs/architext/data/decisions.json`, prepend to `domain-ownership-and-boundaries`'s `consequences`:

```
"Phase 2 LANDED 2026-08-09: 360 lines of pure markdown logic moved from app.js into frontend/dist/src/markdown/ across six modules. Five were leaves with zero dependencies on the rest of app.js, measured from the call graph before the move. Every existing test stayed green and unchanged."
```

Then:

```bash
architext validate .
```

Expected: `Architext validation passed.`

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: drop startEditing's never-false flag, record phase 2

The parameter defaulted to true and all nine call sites passed true explicitly,
so startEditing() was always identical. Its body also read
'started || doc.markdown.length > 0', which with started always true made the
|| dead as well — dead configurability hiding a dead branch behind it.

Architext records the markdown domain modules and the phase."
```

---

## Self-Review

**Spec coverage.** Design section 2 ("command transforms, table operations, fence operations, image tokens and link safety move out of `app.js` into domain modules that take and return strings") → Tasks 1–6. Audit F3 (`startEditing`) → Task 7, in the phase that opens `app.js` as promised. Audit F6 (`wire()`) → deferred with a reason. Design section 3 (Go use cases) → its own plan, stated in Deferred.

**Placeholder scan.** No TBDs. Every task carries its test code, its module header, and its exact verification command. Tasks 1, 2 and 6 say explicitly what to do if a `want` disagrees with the moved code — correct the test, since this is a move and existing behaviour is the specification — rather than leaving the engineer to guess.

**Type consistency.** `tableBounds` returns `{ lines, start, end } | null` in Task 1's Produces and is consumed by `rewriteTable` in the same module. `safeLinkHref` returns `string | null` in Task 4 and `app.js`'s `handleDocumentLinkClick` already branches on `null`. `titleForPath` changes arity in Task 5 and Step 5 of that task says to update every call site. `applyCommand`'s `editorContext` shape (`{ selectionText, blockText }`) matches what `currentEditorContext()` in `app.js` produces and what Task 6's test passes.

**One gap found and closed during review:** Task 3 originally moved `IMAGE_WIDTH_PRESETS` along with the image functions. It is a list of UI preset buttons, not document logic, so Step 3 now says explicitly to leave it in `app.js`.
