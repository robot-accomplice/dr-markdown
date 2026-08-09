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

### 1. `fidelity/` — one owner, one shape

A single port, and a registry that owns the ordering:

```js
// Every preservation implements exactly this.
{ name, capture(originalMarkdown) -> state, restore(serialized, state) -> string }
```

- one module per preservation, each self-contained and pure
- `fidelity/index.js` holds the ordered list — **the one place order lives**
- `editor.js` calls `capture` on build and `restore` on serialize, and knows nothing else about them
- adding a preservation is **one new file and one registry line**, not a three-place edit

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
