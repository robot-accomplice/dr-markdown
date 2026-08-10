# Handover — 2026-08-10

State of play for whoever picks this up next. Written to be actionable without the conversation that
produced it. Supersedes the 2026-08-09 handover entirely; that one described a `main` at v0.4.1 and
three unmerged PRs, none of which is true now.

## Where things stand

**v0.5.0 is released.** `main` is at the v0.5.0 tag, `develop` has nothing `main` lacks, and the
[release](https://github.com/robot-accomplice/dr-markdown/releases/tag/v0.5.0) carries two macOS
DMGs — universal (Intel + Apple Silicon) and arm64-only. Unsigned, un-notarized.

It was the first release to clear the ABORT gate: **five of five GO**, after four consecutive rounds
of five of five NO-GO. The record is `docs/releases/v0.5.0-abort.md` and it is worth reading before
the next release, because it names four residuals that were shipped deliberately.

## What v0.5.0 was for

A data-corruption defect. A GFM footnote definition matched the shape of a link reference definition,
so it was collected as one and re-appended on serialize while the editor also preserved it natively —
and the appended copy was collected again on the next pass. **Files grew by one definition per save,
without bound.** It shipped in 0.4.1 and survived review.

It was found by a 49-construct survey, not by the round-trip corpus and not by reading the code. That
is the most transferable fact in this document: **the corpus only ever contains what somebody already
suspected.**

## The four things that are actually open

1. **Double-clicking a `.md` file does not open it** —
   [#53](https://github.com/robot-accomplice/dr-markdown/issues/53), and the most user-visible defect
   in the product. The bundle declares `CFBundleDocumentTypes`, so macOS routes the file to the app,
   but nothing in `main.go` or `app.go` consumes the launch argument or the open-documents Apple
   Event. You get an empty document. Found by installing the DMG, which four ABORT rounds never did.
2. **No crash handler.** `recover()` appears nowhere. Every non-crash failure now lands in a
   version-stamped event trail beside the preference store; a panic still leaves nothing. Small fix,
   deliberately not made on a frozen release candidate.
3. **Windows and Linux are unbuilt.** Only `tools/build-macos.sh` exists. CI runs the full suite on
   Linux but produces no artifact. A release build matrix is real work — native runners, per-platform
   packaging — not a flag.
4. **The re-serialization class**, accepted since v0.4.0. 38 of 49 surveyed constructs round-trip
   byte-identically and **nothing is deleted any more**, but a construct nobody has surveyed is still
   respelled.

## The architecture, after three refactor phases

Everything below landed against `docs/decisions/2026-08-09-domain-ownership-and-boundaries.md`, an
agreed design plus a clean-code audit. Every pre-existing test stayed green **and unchanged**
throughout, which was the stated judging criterion.

| unit | owns | was |
| --- | --- | --- |
| `frontend/dist/src/fidelity/` | six compensations behind **two ports** and one registry | five ad-hoc pairs on the editor adapter |
| `frontend/dist/src/markdown/` | pure `string -> string` document logic, six modules | 360 lines inside `app.js` |
| `internal/session` | tabs, dirty state, on-disk baseline | loose fields on the Wails binding surface |
| `frontend/dist/src/editor.js` | the Crepe lifecycle, and nothing else — **118 lines**, from 286 | that plus five preservations |

**Two ports, not one, and the distinction matters.** A `Preservation` runs *after* the serializer and
puts captured bytes back. A `SerializerPolicy` runs *before* it and returns options. Forcing markdown
style detection into the Preservation shape would mean a `restore` that does nothing.

**Capture order and restore order are not reverses of each other**, and `fidelity/index.js` says so.
`trailing` must capture first because it reads the original document — frontmatter splitting would
otherwise hand it a body, and a file that is nothing but frontmatter has an empty body whose trailing
run is not the file's.

## Facts that cost real time, and are not obvious from the code

- **`ctx.set` replaces a slice; the serializer holds a reference to the ORIGINAL object.** Mutate what
  the serializer already holds. Verify the *effect*, never the call.
- **Crepe's `config` callbacks run before core slices exist.** Reading one there breaks boot outright —
  a blank window, not an error. Configure after `create()`.
- **mdast source positions ARE reachable**, via `ctx.get('remark')` — not `ctx.get('parser')`, which
  returns ProseMirror nodes with no position data. The old spike concluded "no parser export, so no
  positions"; the premise was true and the conclusion false. Source-preserving editing is therefore no
  longer closed on evidence, but the transaction-to-source mapping is unproven and nothing depends on
  it.
- **Stripping link reference definitions to protect them does not work.** Without its definition
  `[spec][s]` is not a link, so the serializer escapes it. One rewrite for another.
- **Inline HTML is preserved.** `<b>`, `<span>`, `<kbd>`, comments and block `<div>` all round-trip
  byte-identically.

## How to not get burned here

- **`e2e/` is the only coverage of the frontend.** `newTestBrowser` FAILS rather than skips when no
  Chrome is found. `DRMD_SKIP_E2E` is the deliberate opt-out and must never be set in CI.
- **The suite launches one browser per test — 91 per run.** chromedp's 20s connect default was
  exceeded twice on CI; it is raised to 60s in `newTestBrowser`. The real fix is fewer browsers: the
  `fidelity_unit` and `markdown_unit` tests boot a whole browser only to `import()` a pure module and
  could share one.
- **Take the local gate FROM CI, never from memory.** It is
  `gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1`. Eight commits
  once passed a self-defined "vet + test" gate and went red on `gofmt`.
- **Two gates fail in BOTH directions on purpose.** `e2e/fidelity_test.go` and
  `e2e/fidelity_survey_test.go` record what is *still* rewritten, so a fix that is not also disclosed
  in the README breaks the build. If one goes red after a fix, that is the mechanism working.
- **`architext validate` checks schema, not truth.** Six risk records were found describing a codebase
  that no longer existed, and validation passed on every one. Re-read them against the source on every
  release pass.
- **Verify the packaged app, not just the tests.** `wails dev` bridges the production Go bindings to a
  browser, so the binding layer no automated gate covers can be driven directly — that is how v0.5.0
  was confirmed to open a document, report clean, and survive two real saves byte-identically. And
  **install the DMG**: doing that for the first time is what found #53.

## Ground rules that are enforced, not aspirational

- **Modified gitflow is mandatory**: branch → PR to `develop` → PR to `main`. Branch protection now
  requires the `test` check and an up-to-date branch on both, so a red merge is no longer possible.
  Expect a promotion PR to sit `BEHIND` until you merge `main` back into `develop` — that is the
  merge-commit flow working, not a problem.
- Only **robot-accomplice** credentials. `gh` defaults to the wrong account — pin it:
  `export GH_TOKEN=$(gh auth token --user robot-accomplice)`.
- **No Claude/Anthropic attribution** in commits, PRs or artifacts.
- **No Node toolchain.** Not for building, not for testing. `node --check` in CI is a syntax gate only.
- Architext data under `docs/architext/data/**` is the reviewed source of truth and must be updated
  when architecture, trust boundaries or release scope change.

## Release mechanics, in order

1. Bump `wails.json` `productVersion` **and** `appVersion` together — `TestAppVersionMatchesWailsConfig`
   fails on drift.
2. Write the Release Truth record under `docs/architext/data/releases/`, then run `architext doctor .
   --yes` to regenerate the counts rather than hand-writing them. It caught a miscount last time.
   Reconcile and delete the `.bak` it leaves.
3. Run the ABORT stations against the candidate and record the verdicts.
4. `release/vX.Y.Z` → PR to `develop` → PR to `main`.
5. **Rebuild the artifact from the merge commit**, not the release branch:
   `tools/build-macos.sh --universal`. Verify with `lipo -archs` (expect `x86_64 arm64`) and
   `PlistBuddy -c "Print :CFBundleShortVersionString"`.
6. Install the DMG to `/Applications` and open a document with it before publishing.
7. Tag, then `gh release create` with both DMGs.
8. Update the [roboticus-site](https://github.com/robot-accomplice/roboticus-site) project card —
   `src/lib/projects-data.ts` carries `currentVersion` and it went two releases stale last time.
