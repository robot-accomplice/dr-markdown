# Handover — 2026-08-09

State of play for whoever picks this up next. Written to be actionable without the conversation
that produced it.

## ~~Immediate: three PRs are stacked and unmerged~~ — DONE 2026-08-09

All three merged into `develop` in the stated order:

| PR | Branch | What it does |
| --- | --- | --- |
| [#38](https://github.com/robot-accomplice/dr-markdown/pull/38) | `fix/link-reference-definitions` | Stops link reference definitions being deleted |
| [#39](https://github.com/robot-accomplice/dr-markdown/pull/39) | `fix/stale-highlight-race` | Fixes a real render race (stale syntax highlight) |
| [#40](https://github.com/robot-accomplice/dr-markdown/pull/40) | `fix/markdown-style-preservation` | Writes markdown back in the document's own style |

**The hazard this section understated:** #38 and #40 both hook `frontend/dist/src/editor.js`, so they
**conflicted**, and GitHub reported all three `MERGEABLE` right up until #38 landed. A per-PR green
check says nothing about a stack — `mergeable` is computed against `develop` as it is *now*.
`git merge-tree --write-tree <a> <b>` answers it in advance without touching the working tree; use it
before merging any stack here. The conflict itself was two import lines; the fixes compose cleanly
(`#serialize` chains link-refs → alt-text → breaks over disjoint syntax) and `e2e/fidelity_test.go`
passed unchanged with both present, which is the composition check doing its job.

`main` is still at **v0.4.1**; `develop` is now ahead of it by the four fidelity fixes. The next
release is a normal `develop` → `main` promotion; the mechanics are in the v0.4.1 history (bump
`wails.json` **and** `appVersion` — a test fails on drift — then rebuild the artifact from the merge
commit and verify with `strings` before attaching it).

## Where the product actually stands

Released: **v0.4.0** (GA, 2026-08-08) and **v0.4.1** (2026-08-09). Both macOS/arm64 only, unsigned.

The defining constraint is unchanged: **no Node toolchain**. The editor is a vendored Milkdown Crepe
ESM bundle refreshed by `tools/vendor.sh`. Every fidelity fix therefore lives *outside* the bundle,
in `frontend/dist/src/editor.js` and its helpers, so a vendor refresh cannot silently revert them.

### The fidelity blocker: 4 of 5 closed

v0.4.0 shipped over a **NO-GO from all five ABORT stations**, accepted by maintainer decision with
disclosure as the mitigation. The full record is `docs/releases/v0.4.0-abort.md`; the decomposition
is `docs/decisions/2026-08-08-markdown-fidelity-scope.md`.

| # | Problem | Status |
| --- | --- | --- |
| 1 | Inline `<br>` deleted, joining words | **Fixed** (v0.4.1) |
| 2 | CRLF rewritten to LF | **Fixed** (v0.4.1) |
| 3 | Style respellings (bullets, fences, headings, breaks) | **Fixed** (PR #40) |
| 4 | Link reference definitions deleted | **Fixed** (PR #38) |
| 5 | One edit re-serializes the whole document | **Open — architectural** |

Only #5 remains, and it is the one that makes the others a recurring category rather than a finite
list: any construct nobody thought to test is still restyled on save.

## ~~The single next question~~ — ANSWERED 2026-08-09: positions are available

**Task #28: does the parser expose mdast source positions? Yes — via `remark`, not `parser`.**

Probed against a live instance; full record and method in
`docs/decisions/2026-08-08-markdown-fidelity-scope.md`.

- `ctx.get('parser')` returns a **ProseMirror** document — `hasPosition: false`. The question named
  the wrong slice, which is why it stayed open.
- **`ctx.get('remark')` returns the live unified processor**, and `remark.parse(src)` yields **mdast
  with full `position`** — `line`, `column` and `offset`, start and end, on every node down to the
  deepest text leaf.

The old spike's surviving finding — "no parser export, therefore no position data" — was true in its
premise and wrong in its conclusion. The instance hands the processor over by string-addressable
slice regardless of the export surface. **Both legs of the negative have now fallen; option (b) is no
longer closed on evidence.**

**Do not read that as "it works."** Source-preserving editing needs positions *and* a mapping from an
editor transaction back to them. Only the first is established. **The next question is the mapping**,
and it is not a small one. Two recorded limits for whoever takes it: slice names cannot be enumerated
(string addressing only works if you already know the name), and positions are offsets into the
*body*, so they need translating back through the frontmatter split and break sentinels.

## Hard-won facts about this codebase

Things that cost real time to learn and are not obvious from the code:

- **`ctx.set` replaces a slice; the serializer holds a reference to the ORIGINAL object.** A
  replacement is never read. Mutate the object the serializer already holds. Prefer mutating live
  objects over swapping slices, and verify the *effect*, not the call.
- **Crepe's `config` callbacks run before core slices exist.** Reading a slice there breaks boot
  outright. Configure after `create()`.
- **The `<br>` deletion was intentional upstream behaviour**, not a bug: Crepe writes `<br />` to
  mark an empty paragraph and strips those spellings on parse. The match is case-sensitive, which is
  why `<BR>` always survived — that asymmetry is what proved it was narrow rather than architectural.
- **Inline HTML is preserved.** `<b>`, `<span>`, `<kbd>`, comments and block `<div>` all round-trip
  byte-identically. The v0.4.0 notes claimed otherwise and carry a dated correction.
- **Stripping definitions to protect them does not work.** Without its definition `[spec][s]` is not
  a link, so the serializer escapes it to `\[spec]\[s]`. One rewrite for another.

## How to not get burned here

This project has a documented history of tests that could not fail. Four separate instances were
found and fixed: a round-trip corpus comparing a cached string to itself, an e2e helper that skipped
the entire browser suite while `go test` printed `ok`, a font test hardcoding macOS paths that had
only ever passed on its author's machine, and assertions reading the DOM before an async remount.

Consequences for anyone working here:

- **`e2e/` is the only coverage of the frontend.** `newTestBrowser` FAILS rather than skips when no
  Chrome is found; `DRMD_SKIP_E2E` is the deliberate opt-out and must never be set in CI.
- **`testdata/roundtrip/*.canonical.md` must survive byte-identically.** `e2e/fidelity_test.go`
  records what is still rewritten and **fails when that changes in either direction** — if a fix
  makes it go red, update it, that is the mechanism working.
- **Probe edge cases before believing a fix.** Three of the fidelity fixes in this sequence
  introduced a narrower bug of their own, each caught by probing shapes the fixture did not cover,
  not by the fixture.
- **Do not accept "flaky."** The one flake investigated in this sequence turned out to be a real
  product defect that users could hit. Measure before and after (it was 4/20 → 0/20).
- **Contain failures, but never make them invisible.** A `try/catch` logging only to `console.warn`
  hid a `ReferenceError` for two rounds — production builds have no devtools. Route diagnostics
  through the event trail (`bridge.recordEvent`).

## Known open items, none blocking

- **Hard links** are broken by atomic rename. Deliberate: the alternative is the non-atomic write
  the package exists to prevent. Reasoning is in `internal/atomicfile/atomicfile.go`.
- **Large files** are slow (~10 s for 280 KB) with no progress indicator and no cancel.
- **Windows and Linux are unbuilt.** Only `tools/build-macos.sh` exists; the code is platform-aware
  and CI runs on Linux, but no artifact is produced for either.
- **Unsigned and un-notarized** — no Apple Developer account. Disclosed in the README with the
  correct macOS 15+ approval path.
- **Table padding** reflows around content changes; style options cannot express it.

## Ground rules that are enforced, not aspirational

- Modified gitflow is **mandatory**: branch → PR to `develop` → PR to `main`. (One doc commit in this
  session bypassed it and was flagged; don't repeat that.)
- Only **robot-accomplice** credentials. `gh` defaults to the wrong account — pin it:
  `export GH_TOKEN=$(gh auth token --user robot-accomplice)`.
- **No Claude/Anthropic attribution** in commits, PRs or artifacts.
- Architext data under `docs/architext/data/**` is the reviewed source of truth and must be updated
  when architecture, trust boundaries or release scope change. `architext validate` checks **schema,
  not truth** — it passed for four commits while the data was stale, and that is exactly how it was
  missed.
