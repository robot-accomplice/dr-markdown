# Domain ownership and boundaries

**Status: PROPOSED — design for review. No code has been changed for it.**

Date: 2026-08-09

## Why this exists

Five fidelity defects were fixed in two days, and every one of them lived in the same place: the
seam where the vendored editor's output is patched back into the user's own markdown. One of them —
a GFM footnote definition collected as a link reference definition — grew the user's file by one
definition per save, without bound. It shipped, was reviewed, and was found only by surveying
constructs nobody had thought to test.

That is not five unrelated bugs. It is one missing owner, found five times.

## What the code actually looks like

Measured, not estimated.

| unit | size | what it owns |
| --- | --- | --- |
| `frontend/dist/src/app.js` | **2835 lines, 189 top-level functions** | ~10 unrelated domains (below) |
| `app.go` | **747 lines** | the Wails binding surface *and* the application logic behind it |
| `frontend/dist/src/editor.js` | 286 lines | Crepe lifecycle *and* five fidelity preservations |
| `internal/*` | 27–263 lines each | one responsibility each, with clean ports |

### The fidelity seam has no owner

There are five preservations — frontmatter, `<br>` sentinels, link reference definitions, image alt
text, trailing newline. Each is the **same concept**: capture something from the original document,
restore it on the way out. None of them says so. Each is an ad-hoc pair of functions, with its state
on a `WysiwygEditor` private field and its restore step hand-placed in a chain:

```js
#serialize(md) {
  const withRefs = restoreLinkReferences(md, this.#linkRefs)
  const body = restoreBreaks(restoreAltText(withRefs, this.#altByURL), this.#breaks)
  return this.#frontmatter + body.replace(/\n*$/, this.#trailing)
}
```

Adding a sixth means editing **three places** — `#build` to capture, the private field list to hold
state, `#serialize` to restore — and getting the order right by reading the chain. Nothing declares
that these are instances of one thing, so nothing can enumerate them, test them uniformly, or fail
when a new one is added incorrectly. The footnote defect was a capture step that over-matched; there
was no shared shape that would have made "what does this capture, exactly?" a question the design
asks you.

### `app.js` is the framework layer holding all the application logic

Ten domains, identified from its own function names:

document lifecycle and tabs · editor surfaces and mode switching · markdown command transforms ·
table operations · code fences and diagram assistants · image tokens and asset handling · link
safety · preview rendering · preferences and settings · shell chrome, panels and boot

### `app.go` has the right ports and the wrong layering

`newAppWithDependencies` injects `documents`, `native`, `preferences` and `fonts` as interfaces, so
use cases are already testable with fakes and zero infrastructure — `app_staleness_test.go` needed no
filesystem, which is exactly the property the Dependency Rule exists to produce. What is wrong is
that the use cases are methods on the binding surface, so `App` is simultaneously the controller and
the application layer.

## The thesis

**A large part of this product is pure markdown document logic, and almost all of it is currently
embedded in the DOM and Crepe adapter layer.**

Fidelity preservation, command transforms, table operations, fence operations, image token parsing
and link-scheme safety are all `string -> string` or `string -> data`. None of them needs a DOM, a
ProseMirror view, or a Wails binding. They are the *entities and use cases* of a markdown editor, and
they are sitting in the outermost circle.

This matters more here than it would in most projects, because of a constraint this repo already
lives under: **`e2e/` is the only coverage of the frontend.** It takes ~210 s, boots a browser per
test, and its browser launch is nondeterministic in CI. Every pure function above is currently
testable only by booting Chrome. That is not merely slow — it is why the fidelity corpus stayed a
list of constructs someone already suspected, and why a 49-construct survey found defects that
review and the corpus both missed.

Extracting the pure domain is therefore not tidiness. It converts the bulk of this product's logic
from "verifiable only through a browser" to "verifiable directly", without adding a Node toolchain:
a pure module can be exercised by importing it in one already-running page and running a table of
cases, the same mechanism the parser probe used.

## Decision

Adopt domain ownership in three layers, dependencies pointing inward only.

```
frameworks/drivers   Wails bindings · Crepe bundle · DOM · file system
                          |
interface adapters   App (binding surface) · editor.js (Crepe lifecycle)
                     app.js shell (rendering, panels, wiring)
                          |
use cases            open · save · resolve-unsaved · import-asset · apply-command
                          |
entities (pure)      markdown document: fidelity preservations, command
                     transforms, tables, fences, image tokens, link safety
```

### 1. `fidelity/` — one owner, TWO shapes

**Corrected by the clean-code audit below.** An earlier draft of this decision proposed a single
port. That was wrong, and wrong in the way this codebase has been burned by before: it grouped
things that look alike rather than things that change alike.

Five of the six compensations post-process text on the way out. The sixth, style detection, does not
touch the output at all — it configures the serializer *before* it runs, by mutating the live
`remarkStringifyOptions` object. Giving both one port would force a `restore` step that does nothing
onto the one that is not a restore, which is a false abstraction rather than a shared concept.

```js
// Preservation — frontmatter, <br> sentinels, link references, alt text,
// trailing newline. Runs AFTER the serializer, in registry order.
{ name, capture(originalMarkdown) -> state, restore(serialized, state) -> string }

// SerializerPolicy — markdown style. Runs BEFORE the serializer, once per build.
// Reads the document, returns options to apply. No output stage.
{ name, detect(originalMarkdown) -> options }
```

- one module per compensation, each self-contained and pure
- `fidelity/index.js` holds both ordered lists — **the one place order lives**
- `editor.js` runs policies at build and preservations at serialize, and knows nothing else about them
- adding one is **one new file and one registry line**, not a three-place edit
- the two lists are separately enumerable, so "what do we do to the user's bytes, and when" is a
  question the code answers rather than one you reconstruct by reading a chain

### 2. Pure markdown domain

Command transforms, table operations, fence operations, image tokens and link safety move out of
`app.js` into domain modules that take and return strings. `app.js` keeps DOM wiring and calls them.

### 3. Go use cases

`OpenDocument`, `SaveDocument`, `ResolveUnsavedChanges` and asset import become use-case types with
the existing injected ports. `App` becomes a thin adapter that translates Wails calls into use-case
calls. The ports do not change — they are already right.

## Sequencing, and why in this order

1. **`fidelity/` first.** Highest defect density in the codebase by a wide margin, smallest blast
   radius, and it is already fenced by two behavioural gates that fail on any change — the
   byte-identical corpus and the 49-construct survey. The refactor is verifiable as behaviour-preserving
   before it is trusted.
2. **Pure markdown domain second.** Larger, and the tests that would make it safe are precisely the
   ones the extraction enables, so it partly bootstraps its own safety net. Doing it before (1) would
   mean refactoring the highest-risk area with the slowest feedback loop.
3. **Go use cases last.** Real, but the least urgent: dependency inversion is already in place, so
   the defect risk this addresses is comprehension rather than correctness. Deferring it costs
   clarity, not safety — which is the justification for putting it last rather than dropping it.

## Explicitly not doing

- **No Node toolchain, for tests or anything else.** It is the project's defining constraint. Pure
  domain modules are exercised through the existing chromedp harness by importing them directly.
- **No new framework, bundler or DI container.** The registry is a plain array; the ports are plain
  objects and existing Go interfaces.
- **Not decomposing `app.js`'s shell/panel/settings rendering.** That is genuinely view code and
  belongs in the outer circle. Only the pure logic embedded in it moves.
- **Not touching `internal/*`.** Those packages already have single ownership and clean ports, and
  are the model the rest of this follows.

## How this will be judged

The corpus and survey gates must stay green through every step, unchanged. If a refactor step needs
either gate edited to pass, that step changed behaviour and is wrong.

## Risk

The largest is that (2) is a wide edit to a 2835-line file whose only coverage boots a browser. That
is the reason for the ordering above and for treating (2) as a sequence of small extractions, each
landing green, rather than one change.

---

## Clean-code audit, 2026-08-09

Run before turning this design into a plan. Measured across 304 functions in `app.go`, `internal/*`
and `frontend/dist/src/*.js`.

### What is NOT wrong, and should not consume the refactor

Stated first, because a plan that goes looking for these will spend itself on nothing.

| axis | measured | verdict |
| --- | --- | --- |
| function length | **22 of 304 over 30 lines (7%)** | fine |
| parameter lists | **2 functions with >3 params** | fine |
| magic numbers | almost no bare literals; `SAFE_LINK_SCHEMES`, `BREAK_SENTINEL`, `REFUSED_LINK_RECORD_CAP`, `IMPORTABLE_IMAGE_EXTENSIONS` all named | fine |
| returning null | 5 sites; `safeLinkHref`'s is a documented deliberate choice, recorded in Architext | accepted deviation, not debt |

The classic smells are largely absent. This is a disciplined codebase, and the restructure should not
pretend otherwise.

### F1 — One word per concept, violated exactly where the defects clustered

Five instances of one concept, named five ways:

| preservation | capture | restore |
| --- | --- | --- |
| frontmatter | `splitFrontmatter` | *inline string concat — unnamed* |
| `<br>` sentinels | `protectBreaks` | `restoreBreaks` |
| link references | `collectLinkReferences` | `restoreLinkReferences` |
| image alt text | `collectAltText` | `restoreAltText` |
| trailing newline | *inline regex — unnamed* | *inline `.replace` — unnamed* |

**Four different capture verbs — `split`, `protect`, `collect`, `collect` — and two preservations
with no named function on either side.** This is the naming-level evidence for the missing domain,
and it is the mechanism: nothing in the vocabulary says these are the same kind of thing, so nobody
read them as a set, so nobody audited them as a set. The footnote defect was a `collect` that
over-matched, sitting beside a `protect` and a `split` doing the same job under different words.

Fixing the names is not cosmetic here. It is what makes the sixth one obvious.

### F2 — The port design was wrong (see the correction above)

`applyMarkdownStyle` configures the serializer; the other five transform its output. One port would
have forced a no-op `restore` onto the odd one out. Corrected to two ports before any code was
written, which is the value of auditing first.

### F3 — A boolean parameter that is never false

```js
function startEditing(started = true) { … }
```

Defaults to `true`, and **all nine explicit call sites pass `true`**. `startEditing()` is identical.
Dead configurability plus nine redundant flag arguments. Delete the parameter.

### F4 — Unnecessary public surface

`splitFrontmatter` is `export`ed and imported by nobody; it is used only inside `editor.js`. Harmless
today, but it is exactly the kind of accidental public API that a boundary refactor should not carry
forward.

### F5 — Comment density is inversely proportional to complexity

| file | comment ratio | size |
| --- | --- | --- |
| `linkrefs.js` | **51%** | 110 lines |
| `editor.js` | **49%** | 286 lines |
| `atomicfile.go` | 40% | 92 lines |
| `app.go` | 18% | 747 lines |
| **`app.js`** | **5%** | **2835 lines, 189 functions, ~10 domains** |
| `rawmode.js`, `highlighter.js`, `mermaid-renderer.js` | **0%** | 41–92 lines |

The comments this codebase has are the *good* kind — why, not what, recording facts that cost real
debugging. That is a genuine strength and the restructure must carry them across intact rather than
losing them in the move.

But the distribution says the knowledge was written **where someone got burned**, not where a reader
needs it. The single hardest file to understand is the least explained, by an order of magnitude.
That is not an argument for commenting `app.js`; it is an argument that its structure has been
carrying meaning that was never written down anywhere, which is a risk the extraction must respect —
extracting logic whose intent is undocumented means the extractor is guessing.

### F6 — `wire()` is 124 lines, the longest function in the codebase

A flat DOM event-binding table. Length alone is not the problem — a table is allowed to be long —
but it sits in the file with 5% comments and binds handlers across all ten domains, so it is where
the domain boundaries are least visible.

### What this changes about the plan

1. The port design is now two ports, not one — corrected above.
2. **Phase 1 includes a rename to one vocabulary** (`capture`/`restore`), not only a file move. F1
   is the finding that explains the defect cluster, so it is not deferrable polish.
3. F3 and F4 are one-line fixes and can ride along with the phase that touches their file.
4. Do not budget refactor effort for function length, parameter lists or magic numbers.
5. **Every extraction carries its why-comments with it.** For code coming out of `app.js`, where
   there are none, the extraction must document intent as it goes or it is moving code it does not
   understand.
