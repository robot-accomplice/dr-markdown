# Scoping: closing the accepted GA blocker (markdown fidelity)

**Status:** proposal for review. Nothing here is implemented.
**Blocker:** `wysiwyg-lossy-round-trip`, accepted for v0.4.0 GA with disclosure as the mitigation.
**Acceptance test:** `e2e/fidelity_test.go` inverted — every case it currently records as *rewritten*
asserts *preserved*, and `testdata/roundtrip/` grows the fixtures that prove it.

## Summary

The ABORT stations concluded that WYSIWYG editing "rewrites markdown the vendored editor cannot
represent," and the release notes say so. **That framing is partly wrong, and I only found out by
testing the editor instead of trusting the finding.**

Measured against the shipped build:

| Input | Output | Verdict |
| --- | --- | --- |
| `a <b>bold</b> c` | unchanged | **preserved** |
| `a <span>s</span> c` | unchanged | **preserved** |
| `a <kbd>K</kbd> c` | unchanged | **preserved** |
| `a<!-- c -->b` | unchanged | **preserved** |
| `<div>block</div>` | unchanged | **preserved** |
| `a<br>b` | `ab` | **deleted** |
| `a<br/>b` | `ab` | **deleted** |

Inline HTML is **not** the problem. The schema has an inline `html` node carrying a `value`
attribute, and it works for every tag tested except one. `<br>` alone is dropped, and the words on
either side are joined — which is why the damage looked like a general HTML failure.

That reframes the whole blocker. It is not one architectural defect. It is **five separate problems
with very different costs**, and three of them are cheap.

## The five problems, costed

### 1. `<br>` is deleted — SMALL

Not a model gap: `<b>`, `<span>`, `<kbd>`, comments and block HTML all survive the same path. Only
`<br>`/`<br/>` vanish, so something specific to void/break elements is losing it between parse and
serialize — most likely a mapping to a break node that then serializes to nothing.

Worth confirming precisely before fixing, but this is a targeted bug at the adapter layer, not a
reason to replace anything. **It is also the single most destructive item on the list**, because
`<br>` is the standard GFM idiom for a line break inside a table cell, and deletion silently joins
words rather than merely restyling them.

*Effort: hours. Risk: low. Closes the worst user-visible symptom.*

### 2. CRLF rewritten to LF — SMALL

Every line of every Windows-authored file changes on save, producing a whole-file diff. This needs no
editor change at all: detect the dominant line ending when the document is read, and restore it when
serializing — the same seam that already carries frontmatter and image alt text through
`editor.js`'s `#serialize`.

Note this also affects **Raw mode**, where the text control normalizes CRLF per the HTML spec, so
the fix belongs at the document boundary rather than inside the WYSIWYG adapter, and it fixes both
surfaces at once.

*Effort: hours. Risk: low. Fixes the largest-diff complaint.*

### 3. Style respellings — MEDIUM

Bullets `-`/`+` → `*`, setext → ATX, indented code → fenced, `~~~` → ```` ``` ````, `---` → `***`,
closing `##` stripped, table padding reflowed, two-space hard break → `\`.

These come from the serializer's fixed defaults. `mdast-util-to-markdown` exposes options for most
of them (`bullet`, `bulletOrdered`, `fence`, `fences`, `rule`, `setext`, `listItemIndent`,
`incrementListMarker`), and those symbols are present in the vendored bundle — so the knobs exist.

The cost is not turning them on; it is that a *document-wide* option cannot preserve a document with
**mixed** styles, and picking a default silently restyles anyone whose preference differs. The
honest version detects the dominant style per document and configures the serializer to match, which
is more work than it sounds and still loses genuinely mixed usage.

*Effort: days. Risk: medium — a wrong default is a new silent rewrite. Reduces damage; does not
eliminate the class.*

### 4. Link reference definitions deleted — MEDIUM/LARGE

`[spec][s]` is flattened to an inline link and the `[s]: url` definition is removed; an **unused**
definition is deleted outright. This one *is* a real model gap: the schema (`blockquote`,
`hardbreak`, `heading`, `hr`, `html`, `image`, `image-block`, `paragraph`, `table`) has no node for
a definition, so there is nowhere to put it and nothing to serialize back.

Fixing it properly means adding a definition node plus reference-link marks, and keeping them in
sync when links are edited. Deleting content the user cannot see they are losing is the worst
failure mode here — a bibliography of shared URLs disappears with no visible change to the rendered
document.

*Effort: 1–2 weeks. Risk: medium.*

### 5. One edit rewrites the whole file — LARGE, and the only true architectural item

Every change re-serializes the entire document, so the blast radius of one keystroke is the whole
file. Even with 1–4 fixed, any construct nobody has thought to test is still silently restyled on
save. **This is the item that makes the other four a recurring category rather than a finite list.**

Two ways out:

**(a) CommonMark-faithful schema.** Keep the current architecture; close model gaps and pin
serializer options. Incremental, testable, each step ships. But it can only ever converge on
"everything we have tested," and the corpus is the map of what we thought to test — the same
limitation that let all of this ship in the first place.

**(b) Source-preserving editing.** Treat the original text as the source of truth and apply edits as
text patches against it: map an editor transaction back to a source range and splice. Untouched
bytes are never re-serialized, so they cannot be rewritten — this eliminates the class rather than
enumerating it. The parser in the bundle is `micromark`, which is position-tracking by design, so
the offsets needed for this are in principle available.

It is also substantially harder: every editing operation needs a defensible source mapping, and
operations that restructure a document (splitting a list, merging paragraphs) have no single obvious
patch. Realistically a hybrid — patch what maps cleanly, fall back to re-serializing the affected
block — which bounds the blast radius to a block rather than the file.

*Effort: (a) 2–3 weeks incremental. (b) 4–8 weeks, with real design risk.*

## Recommendation

**Do 1 and 2 now, as bug fixes.** They are hours of work, they close the only case that destroys
content rather than restyling it, and they remove the largest diff complaint. They do not depend on
any decision about the engine.

**Then decide between (a) and (b) with a spike, not an argument.** The question that settles it is
whether a source range can be recovered for an ordinary edit in the vendored stack. That is a
day of work to answer and it determines whether (b) is a project or a fantasy. Committing to either
path before answering it would be guessing.

**Do not start 3 or 4 before that spike.** Both are partly wasted if (b) is viable, since
source-preserving editing makes serializer options and definition round-tripping largely moot.

## Constraints any option must respect

- **No Node toolchain.** Any replacement must ship usable prebuilt ESM bundles; the vendored-bundle
  approach is a hard constraint, not a preference.
- **Compensations live outside the bundle.** Frontmatter splitting and alt-text restoration already
  sit in `editor.js` so `tools/vendor.sh` cannot silently revert them. Anything added here should
  follow that, or be a genuine upstream change.
- **The corpus is not a completeness proof.** It records what we thought to test. Every fix here
  should add fixtures, and no fix should be described as closing the class unless it structurally
  cannot regress.

## What I got wrong, recorded

I was ready to write this document recommending an engine replacement, on the strength of three
independent stations agreeing that the editor "cannot represent" the lost constructs. Testing the
editor took twenty minutes and showed inline HTML is preserved almost everywhere — the finding was
real, the diagnosis was not, and the recommendation built on it would have been a multi-week project
aimed at a bug that may be a few hours.

The stations measured *symptoms* correctly and inferred a *cause*. Consensus among reviewers is not
evidence about the code.

---

## Spike result — 2026-08-08

**Question:** can a source range be recovered for an ordinary edit, which decides whether
source-preserving editing (option b) is a project or a fantasy?

**Answer: preliminary NO, on the vendored bundle as shipped.** Recorded with its limits, because this
conclusion is about to remove weeks of work from the plan and it should be overturnable by evidence.

### Established

1. **The bundle exports no parser.** `frontend/dist/vendor/crepe.bundle.mjs` exports exactly
   `Crepe`, `CrepeBuilder`, `CrepeFeature`, `useCrepe`, `useCrepeFeatures`. No remark, no mdast
   utilities, no ProseMirror. Verified by importing it and enumerating the module namespace.
2. **The ProseMirror view is not reachable from the DOM.** `.ProseMirror` carries a `pmViewDesc`
   key, but `pmViewDesc.view` is falsy in this build, so editor state cannot be inspected that way.

### Not established

Whether the `Crepe` *instance* exposes `.editor`/`.ctx`, and through it the view and node
attributes. The probe for that failed on module resolution and I reverted the temporary hook rather
than keep going. **Someone should finish this before treating option (b) as closed.**

### Why the negative holds anyway

Source-preserving editing needs two things: **source positions from the parser**, and a mapping from
an editor transaction back to them. Point 1 removes the first outright — with no parser export there
is no position data to map to, whatever the instance turns out to expose. Obtaining it means
rebundling Milkdown with a wider export surface, which requires a Node toolchain and so collides
with this project's defining constraint, or persuading upstream to export more.

### Revised recommendation

**Take option (a): incremental CommonMark fidelity.** It works entirely through the Crepe
configuration and the `editor.js` seam, both of which are already proven — frontmatter, alt text and
now `<br>` and line endings are all fixed that way, without touching the bundle.

Its known weakness stands and should be stated plainly: it converges on "everything we have tested,"
so the corpus remains the map of what we thought to check. The mitigation is that every fix ships
with fixtures and the characterization test fails when behaviour changes in either direction — which
is how the `<br>` fix was caught updating a stale claim rather than silently diverging from it.

**Reopen option (b) only if** the no-Node constraint is relaxed, or upstream Milkdown exports its
parser. Neither is worth planning around today.

### Next, in order

1. Link reference definitions (the remaining item that *deletes* content) — 1–2 weeks.
2. Serializer style options with per-document detection — days.
3. Re-run the fidelity gate and retire the README caution item by item as each closes.

---

## Spike correction — 2026-08-09

The spike above recorded a **preliminary no** for source-preserving editing and explicitly listed what
it had not established: whether the `Crepe` instance exposes `.editor`/`.ctx`. That probe was left
unfinished, with a note that someone should finish it before treating option (b) as closed.

**It is finished, and the answer changes the picture.** From a live instance:

- `crepe.editor` exists, with `config` (a function) and `ctx`.
- `ctx.get('remarkStringifyOptions')` returns the live options object.
- **`ctx.get('editorView')` returns the ProseMirror view.** The spike's second finding — "the view is
  not reachable" — was true only of the DOM route. It is reachable through the context.
- `ctx.get('serializer')` returns the serializer.

Slices are addressable **by string name**, which is what makes this work without the bundle exporting
any context keys.

So the negative rested on one leg, not two. What survives is the first finding: the bundle exports no
parser, so mdast **source positions** are still not obviously available, and those are the input
source-preserving editing needs. That remains the open question, and it is now the *only* one.

**Someone should probe `ctx.get('parser')` and whether the nodes it produces carry `position` data
before option (b) is called closed.** I have not done that. Recording the correction rather than
leaving a conclusion standing on a finding I have since disproved.

### A working lesson from this, worth keeping

Configuring the serializer took three failed attempts because `ctx.set` **replaces** the slice while
the serializer holds a reference to the original options object — so a replacement is never read.
Mutating the object the serializer already holds is what takes effect. Any future work against this
context should assume the same: prefer mutating live objects over swapping slices, and verify the
effect rather than the call.

The third attempt failed for a different reason entirely — a missing import — and a `try/catch`
written to keep a cosmetic failure from breaking the editor swallowed the `ReferenceError` for two
rounds of debugging. The catch was right; logging only to `console.warn` in a build with no devtools
was not.

---

## Probe result — 2026-08-09: source positions ARE available

The correction above left exactly one question standing, and called it the only one: probe
`ctx.get('parser')` and whether the nodes it produces carry `position` data, before option (b) is
called closed.

**Probed against a live instance. The answer is yes — but not from the slice the question named.**

- **`ctx.get('parser')` is the wrong place.** It returns a function producing a **ProseMirror**
  document: `isProseMirrorNode: true`, `hasPosition: false`, keys `type`/`attrs`/`marks`/`content`.
  Positions were never going to be there. That mis-aimed probe is why the question stayed open.
- **`ctx.get('remark')` returns the live unified processor**, and `remark.parse(src)` yields **mdast
  carrying full `position` data** — `start`/`end` with `line`, `column` and `offset`, on every node
  including the deepest text leaf. Measured on `# Title\n\nSome *emph* here.\n`: the `heading`
  spans offsets 0–7, the trailing `text` leaf 20–26, the `root` 0–27.

So the first finding of the original spike — "the bundle exports no parser, therefore there is no
position data to map to" — was **true in its premise and wrong in its conclusion**. The bundle's
export surface really is just `Crepe`, `CrepeBuilder`, `CrepeFeature`, `useCrepe`,
`useCrepeFeatures`; re-confirmed in this probe. But the *instance* hands over the processor anyway,
by the same string-addressable-slice mechanism that already unlocked `editorView` and `serializer`.
The export surface was never the boundary it was taken for.

Both legs of the negative have now fallen. **Option (b) is no longer closed on evidence.**

### What this does NOT establish

Source-preserving editing needs two things, and this probe delivers one:

1. **Source positions from the parser — ESTABLISHED, available.**
2. **A mapping from an editor transaction back to those positions — still open, untested.**

Nobody should read this as "source-preserving editing works." It removes the blocker that made it
un-plannable; the mapping is the next question and it is not a small one.

Two limits worth recording so the next probe does not re-find them:

- **Slice names could not be enumerated.** Walking the ctx object for a name map returned nothing;
  the container does not expose its keys through any of `sliceMap`/`slices`/`map`. String addressing
  works only if you already know the name, so the reachable surface is still discovered by guessing.
- **Positions would be offsets into the *body*, not the file.** `editor.js` splits frontmatter and
  substitutes break sentinels before Crepe sees the text, so any source range needs translating back
  through those two transforms. A fixed shift, not an obstacle — but it is real and unhandled.

### How this was probed

A temporary test in `e2e/`, since removed: boot the app page, dynamically import the vendored bundle,
create a throwaway Crepe on a detached div, then read the slices off `crepe.editor.ctx`. It is not
kept as a permanent test because it asserts nothing about product behaviour — it answers a question,
and the answer is now written down here.

### Not recorded in Architext, deliberately

`docs/architext/data/decisions.json` holds *accepted* decisions. This is an input to a decision, not
a decision: option (b) is reopened, not chosen. Writing it there would represent an unreviewed
planning proposal as Release Truth, which `CLAUDE.md` explicitly forbids. It belongs here until
somebody decides what to do with it.
