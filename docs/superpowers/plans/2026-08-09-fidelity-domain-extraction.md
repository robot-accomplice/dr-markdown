# Fidelity Domain Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give markdown fidelity a single owner — six compensations behind two declared ports and one registry — so adding one is a new file plus a registry line instead of a three-place edit.

**Architecture:** Five *preservations* transform text after the serializer runs; one *serializer policy* configures the serializer before it runs. They are different shapes and get different ports. A registry in `frontend/dist/src/fidelity/index.js` owns both ordered lists. `editor.js` returns to owning only the Crepe lifecycle. Every module is pure — no DOM, no Crepe, no bridge — so each becomes directly testable in one browser boot instead of one boot per behaviour.

**Tech Stack:** Plain ES modules (no bundler, no Node toolchain). Go 1.26.5 + chromedp for all frontend testing. Vendored Milkdown Crepe 7.22.0.

## Global Constraints

- **No Node toolchain.** Not for building, not for testing. `node --check` in CI is a syntax gate only. Pure modules are exercised by importing them into an already-served page through chromedp.
- **No new framework, bundler, or DI container.** The registry is a plain array; the ports are plain objects.
- **Behaviour-preserving.** This phase changes structure, not behaviour.
- **The judging criterion:** `TestRoundTripCorpus`, `TestFidelitySurveyRewritesExactlyTheseConstructs`, `TestWysiwygRewritesTheseConstructs` and `TestOpeningADocumentDoesNotMarkItModified` must stay green **and unchanged**. A task that needs any of them edited to pass has changed behaviour and is wrong — stop and report rather than editing the gate.
- **Compensations live outside the vendored bundle**, in `frontend/dist/src/`, so `tools/vendor.sh` cannot silently revert them. Never patch `frontend/dist/vendor/`.
- **Carry the comments across verbatim.** `editor.js` and `linkrefs.js` are ~50% comments recording facts that cost real debugging. Moving a function moves its comment block unchanged. Losing one is a regression.
- **Every commit runs the full gate:** `go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1`.
- Attribution rule: no Claude/Anthropic co-author trailers or footers in commits or PRs.

## Ordering is load-bearing — read this before Task 1

Capture order and restore order are **not** reverses of each other. Both were derived from the
pre-refactor chain in `editor.js` and must be reproduced exactly.

Current `#build` captures in this order:

```js
const [frontmatter, rawBody] = splitFrontmatter(markdown)
this.#frontmatter = frontmatter
this.#trailing = markdown.match(/\n*$/)[0]   // <- reads the ORIGINAL, not rawBody
const [body, breaks] = protectBreaks(rawBody)
this.#linkRefs = collectLinkReferences(body)
this.#altByURL = collectAltText(body)
```

Current `#serialize` restores in this order:

```js
const withRefs = restoreLinkReferences(md, this.#linkRefs)
const body = restoreBreaks(restoreAltText(withRefs, this.#altByURL), this.#breaks)
return this.#frontmatter + body.replace(/\n*$/, this.#trailing)
```

So:

- **CAPTURE order:** `trailing, frontmatter, breaks, linkrefs, altText`
  Trailing is moved to **first** so it still reads the original document. It transforms nothing, so
  moving it ahead of the others is behaviour-identical — and it is the only way a sequential runner
  can hand it the original bytes.
- **RESTORE order:** `linkrefs, altText, breaks, frontmatter, trailing`
  Frontmatter is prepended late; trailing governs the final bytes and must be last.

**Do not "simplify" this to a reverse-order stack.** It is not one.

## File Structure

**Create:**

| file | responsibility |
| --- | --- |
| `frontend/dist/src/fidelity/index.js` | The two ports (documented), the two ordered registries, and the runner functions. The one place order lives. |
| `frontend/dist/src/fidelity/trailing.js` | Preservation: the document's trailing newline run. |
| `frontend/dist/src/fidelity/frontmatter.js` | Preservation: YAML frontmatter kept out of the editor model. |
| `frontend/dist/src/fidelity/breaks.js` | Preservation: inline `<br>` spellings via a sentinel. |
| `frontend/dist/src/fidelity/linkrefs.js` | Preservation: link reference definitions and reference syntax. Moved from `src/linkrefs.js`. |
| `frontend/dist/src/fidelity/alttext.js` | Preservation: image alt text the editor overwrites with its resize ratio. |
| `frontend/dist/src/fidelity/style.js` | SerializerPolicy: detect the document's markdown style. Absorbs `src/mdstyle.js`. |
| `e2e/fidelity_unit_test.go` | Direct in-page exercise of every registered module. One browser boot, table-driven. |

**Modify:**

- `frontend/dist/src/editor.js` — shrinks to Crepe lifecycle plus two registry calls.
- `docs/architext/data/nodes.json` — `wysiwyg-editor` sourcePaths and responsibilities.
- `docs/architext/data/decisions.json` — flip `domain-ownership-and-boundaries` to `accepted`.

**Delete:** `frontend/dist/src/linkrefs.js`, `frontend/dist/src/mdstyle.js` (moved, not dropped).

## Deferred from this phase, with reasons

- **`startEditing(started = true)`** (audit F3) lives in `app.js`, which this phase never opens.
  Touching it here would put an unrelated file in the diff. Goes to the Phase 2 plan.
- **Phases 2 and 3** get their own plans. Phase 2's safety net is partly created by this phase, so
  planning it now would be planning against a codebase that does not exist yet.

---

### Task 1: The ports, the registry, and a direct test harness

**Files:**
- Create: `frontend/dist/src/fidelity/index.js`
- Test: `e2e/fidelity_unit_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `PRESERVATIONS: Preservation[]` — capture order.
  - `RESTORE_SEQUENCE: Preservation[]` — restore order.
  - `SERIALIZER_POLICIES: SerializerPolicy[]`
  - `capturePreservations(markdown) -> { markdown: string, states: Map<string, unknown> }`
  - `restorePreservations(serialized, states) -> string`
  - `detectSerializerOptions(markdown) -> object`
  - Port shapes, which every later task implements:
    - `Preservation = { name: string, capture(markdown) -> { state, markdown }, restore(text, state) -> string }`
    - `SerializerPolicy = { name: string, detect(markdown) -> object }`

- [ ] **Step 1: Write the failing test**

Create `e2e/fidelity_unit_test.go`:

```go
package e2e

import (
	"encoding/json"
	"testing"
)

// The fidelity modules are pure — no DOM, no Crepe, no bridge — so they can be
// imported into one already-served page and exercised directly, instead of
// booting a browser per behaviour the way the rest of the frontend suite must.
// That is the point of the extraction, so it gets a test that depends on it.
//
// evalFidelity imports the registry once and runs `expr` with it in scope.
func evalFidelity(t *testing.T, ctx interface{ Done() <-chan struct{} }, expr string, out interface{}) {
	t.Helper()
}

// Every registered module must satisfy its port. A module that half-implements
// one would otherwise fail deep inside a serialize call, with the symptom
// (mangled markdown) far from the cause (a missing restore function).
func TestFidelityRegistryContract(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var out json.RawMessage
	evalJS(t, ctx, `(async () => {
		const f = await import('/src/fidelity/index.js')
		const problems = []
		if (!Array.isArray(f.PRESERVATIONS) || f.PRESERVATIONS.length === 0) problems.push('PRESERVATIONS missing or empty')
		if (!Array.isArray(f.RESTORE_SEQUENCE)) problems.push('RESTORE_SEQUENCE missing')
		if (!Array.isArray(f.SERIALIZER_POLICIES)) problems.push('SERIALIZER_POLICIES missing')
		for (const p of f.PRESERVATIONS || []) {
			if (typeof p.name !== 'string' || !p.name) problems.push('a preservation has no name')
			if (typeof p.capture !== 'function') problems.push(p.name + ': capture is not a function')
			if (typeof p.restore !== 'function') problems.push(p.name + ': restore is not a function')
		}
		for (const p of f.SERIALIZER_POLICIES || []) {
			if (typeof p.name !== 'string' || !p.name) problems.push('a policy has no name')
			if (typeof p.detect !== 'function') problems.push(p.name + ': detect is not a function')
		}
		// Both sequences must contain exactly the same modules. A preservation
		// captured but never restored silently drops the user's bytes.
		const cap = (f.PRESERVATIONS || []).map((p) => p.name).sort().join(',')
		const res = (f.RESTORE_SEQUENCE || []).map((p) => p.name).sort().join(',')
		if (cap !== res) problems.push('capture set != restore set: [' + cap + '] vs [' + res + ']')
		return problems
	})()`, &out)

	var problems []string
	if err := json.Unmarshal(out, &problems); err != nil {
		t.Fatalf("registry probe returned %s: %v", out, err)
	}
	for _, p := range problems {
		t.Errorf("registry contract: %s", p)
	}
}
```

Then delete the unused `evalFidelity` stub above — it was scaffolding; `evalJS` already does the job.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestFidelityRegistryContract -count=1 -v
```

Expected: FAIL — the dynamic import rejects because `/src/fidelity/index.js` does not exist.

- [ ] **Step 3: Create the registry**

Create `frontend/dist/src/fidelity/index.js`:

```js
// The fidelity domain: everything this app does to keep a user's markdown
// byte-faithful across a round trip through the vendored editor.
//
// The editor parses markdown into ProseMirror's model and re-serializes the
// WHOLE document, so anything the model cannot express is rewritten. Each
// module here compensates for one such loss. They used to live as ad-hoc
// function pairs on the editor adapter under four different verbs — split,
// protect, collect, collect — with their restore steps hand-ordered in a
// chain. Nothing said they were the same kind of thing, so nobody read them as
// a set, and a footnote definition collected as a link reference definition
// grew users' files by one copy per save before anyone noticed.
//
// TWO PORTS, because there are genuinely two shapes:
//
//   Preservation      runs AFTER the serializer. Captures something from the
//                     original document and puts it back into the output.
//                     { name, capture(markdown) -> { state, markdown },
//                              restore(text, state) -> string }
//                     capture MAY transform the markdown the editor is given
//                     (frontmatter is removed; break tags are substituted).
//                     Returning the input unchanged is normal.
//
//   SerializerPolicy  runs BEFORE the serializer, once per build. Reads the
//                     document and returns options to apply. It never touches
//                     the output, so it has no restore step. Forcing it into
//                     the Preservation port would mean a restore that does
//                     nothing — a shape that lies about what the module is.
//                     { name, detect(markdown) -> object }

import { trailing } from './trailing.js'
import { frontmatter } from './frontmatter.js'
import { breaks } from './breaks.js'
import { linkReferences } from './linkrefs.js'
import { altText } from './alttext.js'
import { markdownStyle } from './style.js'

// CAPTURE order. Each module receives the previous one's markdown, so a module
// that transforms the text affects what the ones after it see.
//
// `trailing` is FIRST because it must read the original document: frontmatter
// splitting would otherwise hand it a body, and a document that is nothing but
// frontmatter has an empty body whose trailing run is not the file's. It
// transforms nothing, so reading first costs the others nothing.
export const PRESERVATIONS = [trailing, frontmatter, breaks, linkReferences, altText]

// RESTORE order, which is NOT the reverse of capture and is load-bearing. It
// reproduces the hand-written chain this registry replaced, verbatim:
// frontmatter is prepended late, and trailing governs the final bytes so it
// must run last — including after the definition block linkrefs appends.
export const RESTORE_SEQUENCE = [linkReferences, altText, breaks, frontmatter, trailing]

export const SERIALIZER_POLICIES = [markdownStyle]

// capturePreservations runs every capture in order, threading the markdown
// through, and returns the text the editor should be given plus the states to
// hand back to restorePreservations.
export function capturePreservations(markdown) {
  const states = new Map()
  let text = markdown
  for (const preservation of PRESERVATIONS) {
    const { state, markdown: next } = preservation.capture(text)
    states.set(preservation.name, state)
    text = next
  }
  return { markdown: text, states }
}

// restorePreservations puts everything back, in restore order.
export function restorePreservations(serialized, states) {
  let text = serialized
  for (const preservation of RESTORE_SEQUENCE) {
    text = preservation.restore(text, states.get(preservation.name))
  }
  return text
}

// detectSerializerOptions merges every policy's reading of the document. One
// policy today; the merge is here so a second cannot silently overwrite it.
export function detectSerializerOptions(markdown) {
  return SERIALIZER_POLICIES.reduce((options, policy) => Object.assign(options, policy.detect(markdown)), {})
}
```

Create the six modules as minimal stubs so the imports resolve. Each will be filled by its own task:

```bash
cd frontend/dist/src/fidelity
for m in trailing frontmatter breaks linkrefs alttext; do :; done
```

Write each stub file by hand — `trailing.js`:

```js
export const trailing = {
  name: 'trailing',
  capture: (markdown) => ({ state: null, markdown }),
  restore: (text) => text,
}
```

`frontmatter.js` (export name `frontmatter`), `breaks.js` (`breaks`), `linkrefs.js` (`linkReferences`), `alttext.js` (`altText`) follow the identical stub shape with their own export name. `style.js`:

```js
export const markdownStyle = {
  name: 'markdownStyle',
  detect: () => ({}),
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestFidelityRegistryContract -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Verify the gates are untouched**

```bash
go test ./... -count=1
```

Expected: all green. `editor.js` has not changed yet, so behaviour cannot have.

- [ ] **Step 6: Commit**

```bash
git add frontend/dist/src/fidelity e2e/fidelity_unit_test.go
git commit -m "refactor: declare the fidelity ports and registry

Two ports, because there are two shapes: Preservation runs after the serializer
and puts captured bytes back; SerializerPolicy runs before it and returns
options. Forcing one port on both would mean a restore step that does nothing.

Stubs only — no behaviour moved yet. The contract test asserts every registered
module satisfies its port and that the capture and restore sets match, because a
preservation captured but never restored drops the user's bytes silently."
```

---

### Task 2: Move the trailing-newline preservation

**Files:**
- Modify: `frontend/dist/src/fidelity/trailing.js`
- Modify: `frontend/dist/src/editor.js` (remove `#trailing` field, its capture in `#build`, its restore in `#serialize`)
- Test: `e2e/fidelity_unit_test.go`

**Interfaces:**
- Consumes: `Preservation` port from Task 1.
- Produces: `trailing` — `capture` returns `{ state: string, markdown }` where `state` is the trailing newline run of the input.

- [ ] **Step 1: Write the failing test**

Append to `e2e/fidelity_unit_test.go`:

```go
// Each preservation is pure, so it can be driven directly with a table instead
// of through a browser round trip. One boot for the whole table.
func TestTrailingPreservation(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const { trailing } = await import('/src/fidelity/trailing.js')
		const run = (original, serialized) => {
			const { state } = trailing.capture(original)
			return trailing.restore(serialized, state)
		}
		return [
			run('# T\n\n- a\n', '# T\n\n- a\n\n'),       // the list case: extra blank line removed
			run('# T\n', '# T\n'),                         // unchanged stays unchanged
			run('no newline at eof', 'no newline at eof\n'), // a file with no final newline keeps none
			run('# T\n\n\n', '# T\n'),                     // two blank lines at eof are the author's
		]
	})()`, &got)

	want := []string{"# T\n\n- a\n", "# T\n", "no newline at eof", "# T\n\n\n"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestTrailingPreservation -count=1 -v
```

Expected: FAIL — the stub returns text unchanged, so case 0 comes back with the extra blank line.

- [ ] **Step 3: Implement the preservation**

Replace `frontend/dist/src/fidelity/trailing.js`:

```js
// The document's own trailing newline run.
//
// The serializer emits a blank line after a document that ends in a block — a
// list, a footnote block — so every such file gained a line on first edit.
// Trailing blank lines are not content the editor can hold, so the original
// shape is the only faithful answer; this includes a file that ended with no
// newline at all, which is equally the author's choice to keep.
//
// This also fixes a bug that looked unrelated: a list-first document was marked
// modified the moment it opened, because the re-serialized text differed from
// the file by exactly this one newline, so quitting offered to save text the
// user never wrote.
//
// Captured FIRST, so it reads the original document rather than a body with
// frontmatter already removed.
export const trailing = {
  name: 'trailing',
  capture: (markdown) => ({ state: markdown.match(/\n*$/)[0], markdown }),
  restore: (text, state) => text.replace(/\n*$/, state),
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestTrailingPreservation -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Remove the inline version from `editor.js`**

In `frontend/dist/src/editor.js`, delete the `#trailing` field declaration and its comment block, delete `this.#trailing = markdown.match(/\n*$/)[0]` from `#build`, and change the final line of `#serialize` from

```js
    return this.#frontmatter + body.replace(/\n*$/, this.#trailing)
```

to

```js
    return this.#frontmatter + body
```

Then add the registry call. At the top of `editor.js`:

```js
import { capturePreservations, restorePreservations } from './fidelity/index.js'
```

In `#build`, immediately after `const [frontmatter, rawBody] = splitFrontmatter(markdown)` is computed, add:

```js
    this.#states = capturePreservations(markdown)
```

Wait — this is the interleaving step and it is the one place this refactor can go wrong. Do NOT run both the registry and the old inline code. For this task only, call the single moved preservation explicitly:

```js
    this.#trailingState = trailing.capture(markdown).state
```

with `import { trailing } from './fidelity/trailing.js'`, and in `#serialize`:

```js
    return trailing.restore(this.#frontmatter + body, this.#trailingState)
```

Each task moves one module this way. Task 8 replaces all six explicit calls with the two registry calls in one step, once every module is in place.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: all green, **with no gate file edited**. If `TestOpeningADocumentDoesNotMarkItModified` or `TestRoundTripCorpus` fails, the move changed behaviour — revert and re-read the restore ordering section above.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/fidelity/trailing.js frontend/dist/src/editor.js e2e/fidelity_unit_test.go
git commit -m "refactor: move the trailing-newline preservation into fidelity/

First module out. Behaviour identical — the corpus, survey, characterization and
dirty-on-open gates all pass unchanged.

It is now directly testable: four cases run in one browser boot instead of a
round trip per case, which is the whole point of making these modules pure."
```

---

### Task 3: Move the `<br>` sentinel preservation

**Files:**
- Modify: `frontend/dist/src/fidelity/breaks.js`
- Modify: `frontend/dist/src/editor.js` (remove `STRIPPED_BREAKS`, `BREAK_SENTINEL`, `BREAK_LIKE`, `protectBreaks`, `restoreBreaks`, `#breaks`)
- Test: `e2e/fidelity_unit_test.go`

**Interfaces:**
- Consumes: `Preservation` port.
- Produces: `breaks` — `capture(markdown)` returns `{ state: string[], markdown }` where `markdown` has every stripped break spelling replaced by the sentinel.

- [ ] **Step 1: Write the failing test**

Append to `e2e/fidelity_unit_test.go`:

```go
func TestBreaksPreservation(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const { breaks } = await import('/src/fidelity/breaks.js')
		const roundTrip = (md) => {
			const { state, markdown } = breaks.capture(md)
			return breaks.restore(markdown, state)
		}
		const captured = breaks.capture('a<br>b<br />c\n')
		return [
			roundTrip('a<br>b\n'),
			roundTrip('a<br />b<br>c\n'),
			roundTrip('a<BR>b\n'),
			captured.markdown,
			roundTrip('a<br  >b\n'),
		]
	})()`, &got)

	want := []string{
		"a<br>b\n",             // exact spelling restored
		"a<br />b<br>c\n",      // several spellings restored in document order
		"a<BR>b\n",             // uppercase was never stripped; must pass through untouched
		"a<br  >b<br  >c\n",    // capture substitutes the same-length sentinel
		"a<br  >b\n",           // a document that already spelled it that way keeps its slot
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
go test ./e2e -run TestBreaksPreservation -count=1 -v
```

Expected: FAIL — the stub passes text through, so case 3 returns the input rather than sentinels.

- [ ] **Step 3: Implement the preservation**

Replace `frontend/dist/src/fidelity/breaks.js` with the constants and both functions moved **verbatim** from `editor.js`, comments included, wrapped in the port. Note the duplicated comment block above `STRIPPED_BREAKS` in the current file — two drafts of the same paragraph. Keep the second (it describes all five spellings); drop the first.

```js
// The exact spellings the vendored editor strips from the parsed document,
// plus the double-space variant used to carry them through it. Taken from the
// bundle, which matches `html` nodes whose trimmed value is one of the first
// four — case-sensitively, which is why `<BR>` survives untouched.
const STRIPPED_BREAKS = ['<br>', '<br/>', '<br />', '<br >']
const BREAK_SENTINEL = '<br  >'
const BREAK_LIKE = /<br\s*\/?\s*>/g

// Crepe writes `<br />` to represent an empty paragraph, and strips those same
// spellings when parsing so its own marker round-trips. Genuine `<br>` written
// by the user is collateral: it is removed from the document and the words on
// either side are joined, so `run make<br>then sign` becomes `run makethen
// sign`. That is content destruction rather than restyling, and `<br>` is the
// standard GFM idiom for a line break inside a table cell.
//
// Every break-like tag is swapped for the double-space spelling, which is not
// in the stripped set and therefore survives, and the originals are restored in
// document order on the way out — so the user gets back the exact form they
// wrote. The sentinel is deliberately the same length as the longest spelling
// it stands in for: a longer placeholder inflates the column padding the
// serializer computes for a table, turning a content fix into a layout rewrite.
//
// A break the user's own document already spelled `<br  >` is queued too, so it
// cannot be mistaken for a sentinel and consume another break's slot.
//
// TRADE-OFF, deliberate: while editing, the tag shows as a literal rather than
// as a line break. That is already how this editor displays every other inline
// tag — `<span>` and `<kbd>` render as literal text too — so it is consistent
// with the surrounding behaviour rather than a new wart, and it replaces silent
// deletion with something visible and reversible. The real fix is schema-level
// and is scoped in docs/decisions/2026-08-08-markdown-fidelity-scope.md.
//
// Known limit, shared with alt-text restoration: adding or removing a break
// inside the editor shifts the remaining originals by one, because the queue is
// positional. Every spelling is still a valid line break, so the failure is a
// changed spelling rather than lost content.
export const breaks = {
  name: 'breaks',
  capture(markdown) {
    const state = []
    const substituted = markdown.replace(BREAK_LIKE, (match) => {
      if (!STRIPPED_BREAKS.includes(match) && match !== BREAK_SENTINEL) return match
      state.push(match)
      return BREAK_SENTINEL
    })
    return { state, markdown: substituted }
  },
  restore(text, state) {
    const queue = [...state]
    return text.replaceAll(BREAK_SENTINEL, () => queue.shift() ?? '<br>')
  },
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestBreaksPreservation -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Remove the inline version from `editor.js`**

Delete `STRIPPED_BREAKS`, `BREAK_SENTINEL`, `BREAK_LIKE`, `protectBreaks`, `restoreBreaks` and their comment blocks. Change `#build`:

```js
    const captured = breaks.capture(rawBody)
    this.#breakState = captured.state
    const body = captured.markdown
```

and in `#serialize`, replace `restoreBreaks(..., this.#breaks)` with `breaks.restore(..., this.#breakState)`. Import `breaks` from `./fidelity/breaks.js`. Rename the `#breaks` field to `#breakState` for consistency with `#trailingState`.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green, no gate file edited. `TestWysiwygRewritesTheseConstructs/table_padding_reflows_around_a_preserved_br` is the sharpest check here — it fails if the sentinel length changes.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/fidelity/breaks.js frontend/dist/src/editor.js e2e/fidelity_unit_test.go
git commit -m "refactor: move the <br> sentinel preservation into fidelity/

Verbatim move, comments included, wrapped in the Preservation port. Also drops a
duplicated comment block left over from two drafts of the same paragraph.

Directly tested now, including the two cases the old design made awkward to
reach: <BR> passing through untouched, and a document that already spelled a
break as the sentinel keeping its own slot."
```

---

### Task 4: Move the frontmatter preservation

**Files:**
- Modify: `frontend/dist/src/fidelity/frontmatter.js`
- Modify: `frontend/dist/src/editor.js` (remove `FRONTMATTER`, `splitFrontmatter`, `#frontmatter`)
- Test: `e2e/fidelity_unit_test.go`

**Interfaces:**
- Consumes: `Preservation` port.
- Produces: `frontmatter` — `capture` returns `{ state: string, markdown }` where `markdown` is the body with frontmatter removed and `state` is the frontmatter block verbatim.

Note: `splitFrontmatter` is currently `export`ed with no importer (audit F4). It becomes internal here — do not re-export it.

- [ ] **Step 1: Write the failing test**

```go
func TestFrontmatterPreservation(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const { frontmatter } = await import('/src/fidelity/frontmatter.js')
		const doc = '---\ntitle: T\n---\n\n# Body\n'
		const c = frontmatter.capture(doc)
		const noFm = frontmatter.capture('# Body\n')
		return [
			c.markdown,
			frontmatter.restore(c.markdown, c.state),
			noFm.markdown,
			frontmatter.restore(noFm.markdown, noFm.state),
		]
	})()`, &got)

	want := []string{
		"# Body\n",                        // the editor never sees the frontmatter
		"---\ntitle: T\n---\n\n# Body\n",  // and it comes back byte-identical
		"# Body\n",                        // a document without frontmatter is untouched
		"# Body\n",
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
go test ./e2e -run TestFrontmatterPreservation -count=1 -v
```

Expected: FAIL — the stub returns the document with frontmatter still attached.

- [ ] **Step 3: Implement the preservation**

Replace `frontend/dist/src/fidelity/frontmatter.js`, moving the regex and its comments verbatim:

```js
// YAML frontmatter, only at the very start of the document.
// Trailing blank lines are part of the captured block: the editor trims
// leading blank lines from its body, so leaving the separator behind would
// silently close the gap between frontmatter and the first heading.
const FRONTMATTER = /^---\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n(?:[ \t]*\r?\n)*)?/

// Crepe parses `---` as a thematic break and re-serializes the block as `***`
// followed by a setext heading, so a single edit silently destroyed the title,
// date, tags and draft status of every Hugo, Jekyll, Obsidian and Astro
// document. Markdown on disk is this product's source of truth; bytes the
// editor cannot represent must not pass through it.
export const frontmatter = {
  name: 'frontmatter',
  capture(markdown) {
    const match = markdown.match(FRONTMATTER)
    if (!match) return { state: '', markdown }
    return { state: match[0], markdown: markdown.slice(match[0].length) }
  },
  restore: (text, state) => state + text,
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestFrontmatterPreservation -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Remove the inline version from `editor.js`**

Delete `FRONTMATTER`, `splitFrontmatter` and the `#frontmatter` field. In `#build` use `frontmatter.capture(markdown)` for the body; in `#serialize` use `frontmatter.restore(...)`. Import from `./fidelity/frontmatter.js`.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. `testdata/roundtrip/09-frontmatter.canonical.md` and `10-frontmatter-rich.canonical.md` are the sharp checks.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/fidelity/frontmatter.js frontend/dist/src/editor.js e2e/fidelity_unit_test.go
git commit -m "refactor: move the frontmatter preservation into fidelity/

Verbatim move with its comments. splitFrontmatter was exported with no importer;
it is internal to the module now rather than accidental public API."
```

---

### Task 5: Move the alt-text preservation

**Files:**
- Modify: `frontend/dist/src/fidelity/alttext.js`
- Modify: `frontend/dist/src/editor.js` (remove `IMAGE`, `RATIO_ALT`, `collectAltText`, `restoreAltText`, `#altByURL`)
- Test: `e2e/fidelity_unit_test.go`

**Interfaces:**
- Consumes: `Preservation` port.
- Produces: `altText` — `capture` returns `{ state: Map<string, string[]>, markdown }` with `markdown` unchanged.

- [ ] **Step 1: Write the failing test**

```go
func TestAltTextPreservation(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const { altText } = await import('/src/fidelity/alttext.js')
		// The editor replaces alt text with its resize ratio, so 'serialized'
		// below is what comes back out of it.
		const run = (original, serialized) =>
			altText.restore(serialized, altText.capture(original).state)
		return [
			run('![Diagram](a.png)\n', '![1.00](a.png)\n'),
			run('![Before](x.png)\n\n![After](x.png)\n', '![1.00](x.png)\n\n![1.00](x.png)\n'),
			run('![3.14](pi.png)\n', '![1.00](pi.png)\n'),
			run('![alt with [x] inside](b.png)\n', '![1.00](b.png)\n'),
		]
	})()`, &got)

	want := []string{
		"![Diagram](a.png)\n",                        // the caption comes back
		"![Before](x.png)\n\n![After](x.png)\n",      // one URL, two captions, document order
		"![3.14](pi.png)\n",                          // a genuinely numeric alt is content, not a ratio
		"![alt with [x] inside](b.png)\n",            // bracketed alts survive
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
go test ./e2e -run TestAltTextPreservation -count=1 -v
```

Expected: FAIL — the stub returns the serialized text, so every case still shows `![1.00]`.

- [ ] **Step 3: Implement the preservation**

Move `IMAGE`, `RATIO_ALT`, `collectAltText` and `restoreAltText` from `editor.js` into `frontend/dist/src/fidelity/alttext.js` verbatim with their full comment blocks, wrapped as:

```js
export const altText = {
  name: 'altText',
  capture: (markdown) => ({ state: collectAltText(markdown), markdown }),
  restore: (text, state) => restoreAltText(text, state),
}
```

Keep `collectAltText` and `restoreAltText` as module-private functions so the moved comments stay attached to the code they describe.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestAltTextPreservation -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Remove the inline version from `editor.js`**

Delete `IMAGE`, `RATIO_ALT`, `collectAltText`, `restoreAltText` and `#altByURL`. Wire `altText.capture` / `altText.restore` as in the previous tasks.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. Fixtures `11-image`, `12-image-repeated`, `13-image-numeric-alt`, `14-image-bracket-alt` are the sharp checks — each exists because a previous version of this fix broke that case.

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/src/fidelity/alttext.js frontend/dist/src/editor.js e2e/fidelity_unit_test.go
git commit -m "refactor: move the alt-text preservation into fidelity/

Verbatim move with comments. The four direct cases mirror the four roundtrip
fixtures, each of which exists because an earlier version of this fix broke that
shape — repeated URLs, numeric alts, bracketed alts."
```

---

### Task 6: Move the link-reference preservation

**Files:**
- Create: `frontend/dist/src/fidelity/linkrefs.js` (content moved from `frontend/dist/src/linkrefs.js`)
- Delete: `frontend/dist/src/linkrefs.js`
- Modify: `frontend/dist/src/editor.js` (remove the import and `#linkRefs`)
- Test: `e2e/fidelity_unit_test.go`

**Interfaces:**
- Consumes: `Preservation` port.
- Produces: `linkReferences` — `capture` returns `{ state, markdown }` with `markdown` unchanged; `state` is `null` when the document has no definitions.

- [ ] **Step 1: Write the failing test**

```go
func TestLinkReferencePreservation(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const { linkReferences } = await import('/src/fidelity/linkrefs.js')
		// 'serialized' is what the editor produces: references inlined,
		// definitions dropped.
		const run = (original, serialized) =>
			linkReferences.restore(serialized, linkReferences.capture(original).state)
		return [
			run('See the [spec][s].\n\n[s]: https://x/spec\n', 'See the [spec](https://x/spec).\n'),
			run('A claim[^src].\n\n[^src]: Ibid.\n', 'A claim[^src].\n\n[^src]: Ibid.\n'),
			run('No refs here.\n', 'No refs here.\n'),
		]
	})()`, &got)

	want := []string{
		"See the [spec][s].\n\n[s]: https://x/spec\n", // reference syntax and definition both restored
		"A claim[^src].\n\n[^src]: Ibid.\n",           // a GFM footnote is NOT a link reference
		"No refs here.\n",                             // no definitions means no state and no change
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}
```

The second case is the regression that grew users' files by one definition per save. It is here rather than only in the corpus because this is where it can be read.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestLinkReferencePreservation -count=1 -v
```

Expected: FAIL — the module does not exist at that path yet.

- [ ] **Step 3: Move the module**

```bash
git mv frontend/dist/src/linkrefs.js frontend/dist/src/fidelity/linkrefs.js
```

Then append the port wrapper to the moved file, keeping `collectLinkReferences` and `restoreLinkReferences` as module-private functions with their comments intact:

```js
export const linkReferences = {
  name: 'linkReferences',
  capture: (markdown) => ({ state: collectLinkReferences(markdown), markdown }),
  restore: (text, state) => restoreLinkReferences(text, state),
}
```

Remove the `export` keyword from `collectLinkReferences` and `restoreLinkReferences` — the port is the module's only public surface now.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestLinkReferencePreservation -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Remove the old wiring from `editor.js`**

Delete `import { collectLinkReferences, restoreLinkReferences } from './linkrefs.js'` and the `#linkRefs` field; wire `linkReferences.capture` / `.restore` as in previous tasks.

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. Fixtures `17-link-refs`, `18-linkref-unused-only`, `19-linkref-mixed` and `21-footnote` are the sharp checks.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move link-reference preservation into fidelity/

git mv plus a port wrapper; collect/restore become module-private so the port is
the only public surface.

The footnote case is a direct test now. It is the defect that grew a user's file
by one definition per save, and it was found by a broad survey rather than by
the corpus or by review — so it gets a test where a reader will see it."
```

---

### Task 7: Move the markdown-style serializer policy

**Files:**
- Create: `frontend/dist/src/fidelity/style.js` (content moved from `frontend/dist/src/mdstyle.js`)
- Delete: `frontend/dist/src/mdstyle.js`
- Modify: `frontend/dist/src/editor.js` (remove `STYLE_DEFAULTS`, `applyMarkdownStyle`)
- Test: `e2e/fidelity_unit_test.go`

**Interfaces:**
- Consumes: `SerializerPolicy` port.
- Produces: `markdownStyle` — `detect(markdown) -> object` returning a full options object (defaults merged with what the document expressed).

The defaults move into this module. Applying only detected keys would let a style set for one document persist into the next if the options object were ever shared, so `detect` returns every key on every call.

- [ ] **Step 1: Write the failing test**

```go
func TestMarkdownStylePolicy(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const { markdownStyle } = await import('/src/fidelity/style.js')
		const k = (md, key) => String(markdownStyle.detect(md)[key])
		return [
			k('- a\n- b\n', 'bullet'),
			k('* a\n* b\n', 'bullet'),
			k('# H #\n', 'closeAtx'),
			k('# H\n', 'closeAtx'),
			k('1. a\n1. b\n', 'incrementListMarker'),
			k('1. a\n2. b\n', 'incrementListMarker'),
			k('-\ta\n-\tb\n', 'bullet'),
		]
	})()`, &got)

	want := []string{"-", "*", "true", "false", "false", "true", "-"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}
```

The last case is the tab-separated bullet: detection counted only space-separated markers, so a tab-indented document expressed no preference and every bullet in it was rewritten to the default `*`.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestMarkdownStylePolicy -count=1 -v
```

Expected: FAIL — the stub `detect` returns `{}`, so every lookup is `undefined`.

- [ ] **Step 3: Move the module**

```bash
git mv frontend/dist/src/mdstyle.js frontend/dist/src/fidelity/style.js
```

Move `STYLE_DEFAULTS` out of `editor.js` into `style.js`, keeping its comment about writing every key on every build. Make `detectMarkdownStyle` module-private and append:

```js
export const markdownStyle = {
  name: 'markdownStyle',
  detect: (markdown) => ({ ...STYLE_DEFAULTS, ...detectMarkdownStyle(markdown) }),
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestMarkdownStylePolicy -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Rewire `editor.js`**

Replace `applyMarkdownStyle` with a call to the registry's `detectSerializerOptions`, keeping the try/catch and both diagnostics — the console line and `bridge.recordEvent`. That catch swallowed a `ReferenceError` for two rounds of debugging; it must keep routing through the event trail.

```js
function applySerializerOptions(crepe, markdown) {
  try {
    const options = crepe.editor?.ctx?.get('remarkStringifyOptions')
    if (!options) return
    // The options object must be MUTATED, not replaced. `ctx.set` swaps the
    // slice value, but the serializer captured a reference to the original
    // object when it was built, so a replacement is simply never read.
    Object.assign(options, detectSerializerOptions(markdown))
  } catch (error) {
    // A style mismatch is a cosmetic diff; letting it break the editor would
    // trade a formatting nuisance for an unusable document. The catch is right;
    // logging only to console was not — a production build has no devtools.
    console.warn('editor: could not apply document markdown style', error)
    bridge.recordEvent('editor.style-failed', { error: String(error?.message ?? error) })
  }
}
```

- [ ] **Step 6: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. `testdata/roundtrip/20-style.canonical.md` and `22-style-atx-ordered.canonical.md` are the sharp checks, plus the survey's `atx closed` and `ordered no increment` entries.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move markdown style detection into fidelity/ as a policy

It is a SerializerPolicy, not a Preservation: it configures the serializer
before it runs rather than transforming its output, so it has no restore step.
Giving it the Preservation port would have meant a restore that does nothing.

STYLE_DEFAULTS moves with it. detect() returns every key on every call, because
applying only the detected ones would let one document's style persist into the
next if the options object were ever shared."
```

---

### Task 8: Collapse `editor.js` onto the registry

**Files:**
- Modify: `frontend/dist/src/editor.js`
- Modify: `docs/architext/data/nodes.json`
- Modify: `docs/architext/data/decisions.json`

**Interfaces:**
- Consumes: `capturePreservations`, `restorePreservations`, `detectSerializerOptions` from Task 1.
- Produces: `WysiwygEditor` with a single `#states` field replacing the five per-preservation fields.

- [ ] **Step 1: Write the failing test**

Append to `e2e/fidelity_unit_test.go`. This asserts the outcome of the whole phase — that the editor no longer knows any preservation by name:

```go
// The point of the phase: editor.js owns the Crepe lifecycle and nothing else.
// If it still names an individual preservation, the registry is not the single
// owner and adding the next compensation is still a multi-place edit.
func TestEditorDoesNotKnowIndividualPreservations(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "frontend", "dist", "src", "editor.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"splitFrontmatter", "protectBreaks", "restoreBreaks",
		"collectAltText", "restoreAltText",
		"collectLinkReferences", "restoreLinkReferences",
		"STYLE_DEFAULTS", "BREAK_SENTINEL", "FRONTMATTER", "RATIO_ALT",
	} {
		if strings.Contains(string(source), name) {
			t.Errorf("editor.js still references %q; it belongs to the fidelity registry now", name)
		}
	}
}
```

Add `"os"`, `"path/filepath"` and `"strings"` to the file's imports.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./e2e -run TestEditorDoesNotKnowIndividualPreservations -count=1 -v
```

Expected: FAIL — after Tasks 2–7 the editor still calls each module explicitly by name.

- [ ] **Step 3: Collapse onto the registry**

In `editor.js`, replace the six explicit imports with one:

```js
import { capturePreservations, restorePreservations, detectSerializerOptions } from './fidelity/index.js'
```

Replace the five per-preservation fields with:

```js
  // Everything the fidelity registry captured from the document as opened,
  // keyed by preservation name. The editor does not know what is in here.
  #states = new Map()
```

`#build` becomes:

```js
  async #build(host, markdown) {
    const captured = capturePreservations(markdown)
    this.#states = captured.states
    const body = captured.markdown
    host.replaceChildren()
    const crepe = new Crepe({ /* unchanged */ })
    crepe.on(/* unchanged */)
    await crepe.create()
    applySerializerOptions(crepe, body)
    this.#crepe = crepe
    this.#baseline = crepe.getMarkdown()
  }
```

`#serialize` becomes:

```js
  // The single exit for markdown leaving the editor. Applied on the way out
  // only — #baseline still compares raw Crepe output, so change detection is
  // unaffected.
  #serialize(md) {
    return restorePreservations(md, this.#states)
  }
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./e2e -run TestEditorDoesNotKnowIndividualPreservations -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Run the full gate**

```bash
go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: all green, **with no gate file edited**. This is the step where an ordering mistake surfaces; if a fixture fails, re-read the ordering section rather than adjusting the fixture.

- [ ] **Step 6: Update the architecture data**

In `docs/architext/data/nodes.json`, for node `wysiwyg-editor`: replace the four compensation responsibilities with a single entry —

```
"Run the fidelity registry: capture preservations from the document on build, apply serializer policies, and restore preservations on serialize"
```

— and set `sourcePaths` to `frontend/dist/src/editor.js`, `frontend/dist/src/fidelity/index.js`, `frontend/dist/vendor/crepe.bundle.mjs`. Add the new tests to `verification`.

In `docs/architext/data/decisions.json`, change `domain-ownership-and-boundaries` status from `planned` to `accepted`, and note in `consequences` that phase 1 landed.

```bash
architext validate .
```

Expected: `Architext validation passed.`

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: editor.js owns the Crepe lifecycle and nothing else

Five per-preservation fields and a hand-ordered restore chain collapse into one
#states map and two registry calls. The editor no longer knows any compensation
by name, which a test now enforces — that was the whole point, since the defect
this phase came from was a capture step nobody recognised as one of a set.

Adding the next compensation is one file and one registry line.

Architext updated: the wysiwyg-editor node now records the registry rather than
enumerating compensations, and the decision moves from planned to accepted."
```

---

## Self-Review

**Spec coverage.** Design section 1 (`fidelity/`, two ports, registry owning order) → Tasks 1–8. Audit F1 (one vocabulary) → every module now uses `capture`/`restore`; Task 8 enforces it. Audit F2 (two ports) → Task 1 and Task 7. Audit F4 (`splitFrontmatter` export) → Task 4 Step 3. Audit F5 (carry comments) → Global Constraints, and every move step says "verbatim, comments included". Design sections 2 and 3 → deferred to their own plans, with reasons recorded above. Audit F3 (`startEditing`) → deferred to Phase 2, with a reason. Audit F6 (`wire()`) → Phase 2; it is in `app.js`, untouched here.

**Placeholder scan.** No TBDs. Every code step carries the actual code. Task 1's stub-file step names each file and its export explicitly rather than saying "and the others similarly".

**Type consistency.** Export names used consistently across tasks: `trailing`, `frontmatter`, `breaks`, `linkReferences`, `altText` (all `Preservation`), `markdownStyle` (`SerializerPolicy`). Registry functions: `capturePreservations`, `restorePreservations`, `detectSerializerOptions` — same names in Task 1's Produces block, Task 8's Consumes block, and Task 8's code. Field names: `#trailingState` and `#breakState` are introduced in Tasks 2 and 3 and both removed in Task 8 when `#states` replaces them.

**One correction found during review:** Task 1's test file originally declared an `evalFidelity` helper it never used. Step 1 now says to delete it — an unused helper in a test file is the kind of thing that gets copied.
