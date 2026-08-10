# Handover — 2026-08-10 (rev 3)

State of play for whoever picks this up next. Written to be actionable without the conversation that
produced it. Supersedes the 2026-08-09 handover entirely; that one described a `main` at v0.4.1 and
three unmerged PRs, none of which is true now.

Rev 3 corrects a claim rev 2 got wrong rather than merely stale: it said there was no crash handler
and that a panic left nothing behind. The first half was true of this repo and the second was false
of the running program, because the recovery lives in Wails. Open item 2 now records what actually
happens. **The general lesson is worth more than the correction: a grep over your own source cannot
settle a question about your framework's behaviour.**

## Where things stand

**v0.5.0 is released, and `develop` has already moved past it.** `main` is at the v0.5.0 tag and the
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

## Waiting to ship: v0.5.1

**`release/v0.5.1` is cut and carries two user-facing fixes that are not released.** v0.5.0 on `main`
still has both defects. Versions are bumped in `wails.json` and `app.go`, and the Release Truth
record is written.

- **Finder file-open is fixed** — [#53](https://github.com/robot-accomplice/dr-markdown/issues/53),
  PR #56. `main.go` now sets `mac.Options.OnFileOpen`. Two arrival paths, because there are two
  situations: at launch the file reaches Go before the webview exists, so `App` holds it and the
  frontend asks through `FrontendReady`; while running, Go emits `file:open` and the frontend listens.
  Verified on the installed `.app` for both paths.
- **Panics are recorded and shown** — PR #59. `App.reportPanic` writes operation, message, stack and
  build version into the event trail, shows a dialog naming the operation, then re-panics. It must
  not recover: the guard runs during unwinding, so swallowing would return the method's zero values
  and tell the frontend a failed save succeeded. Installed on all 18 bound methods and on `startup`,
  `beforeClose` and `openFileFromOS`. There is no seam to install it centrally — Wails binds by
  reflection and `ErrorFormatter` is bypassed, because a panic unwinds past the line that calls it —
  so **coverage is enforced by two source-parsing gates** that fail when a method arrives unguarded.
- Cutting v0.5.1 is the mechanics at the end of this document, unchanged. `main` is already merged
  into the release branch, so the promotion PR should not sit `BEHIND`.

## The things that are actually open

1. **The app has no File or View menu** —
   [#57](https://github.com/robot-accomplice/dr-markdown/issues/57). Rev 3 of this document said the
   app had **no menu bar at all** and that `Cmd+Q` most likely did not quit. Both are false, and the
   correction came from driving the installed v0.5.1 DMG on 2026-08-10.
   The app **does** have `Dr. Markdown`, `Edit` and `Window` menus, and the app menu contains
   **`Quit Dr. Markdown ⌘Q`**, which was used to quit it. Every source fact behind the old claim was
   correct — `main.go` passes no `Menu`, and Wails v2.13.0 does leave `DefaultMacMenu()` commented
   out — and the conclusion was still wrong, because **AppKit supplies a default menu bar itself when
   an application sets none**. The old note even said "established from source; not driven", which was
   the warning that should have been acted on.
   What is genuinely missing is **File** (New, Open, Save, Save As), **View** (where the ribbon design
   work wanted an item), and About/Settings in the app menu. That is smaller than building a menu bar:
   it is adding this app's own commands on top of a default that already handles the system ones.
   Still a subsystem — Go menu construction, dispatch into the frontend, checkmark state — and the
   chromedp harness still cannot verify it, because a native menu does not exist in a browser view.
   **The lesson generalises past menus: this is the second time in one day that reasoning about a
   platform's behaviour from this repository's own source produced a confident false conclusion.**
   The other was `recover()` (open item 2). Drive the packaged app.
2. **A panicking bound call never settles its frontend promise.** This replaces the old item, which
   said there was no crash handler and that a panic left nothing behind. `recover()` appearing
   nowhere in this repo was a true grep and a false conclusion: Wails recovers panics inside
   bound-method dispatch, so the process always survived one. It then returns an empty result,
   `darwin/frontend.go` hands that empty string to `Frontend.Callback`, and the runtime's `Callback`
   throws on `JSON.parse("")` before it reaches the pending callback — whose timeout is never armed,
   because Wails arms one only when a caller passes a positive timeout. **The `await` never settles
   and that operation is dead until restart.** As of 0.5.1 the panic is recorded and a dialog names
   the operation, which is the only report the user can get; the hang itself is a Wails defect that
   re-panicking preserves rather than fixes. Closing it means `DisablePanicRecovery` (a dead process
   instead of a dead call) or a frontend call timeout (changes every binding). Tracked as
   [#61](https://github.com/robot-accomplice/dr-markdown/issues/61); the `wails-exit-path` roadmap
   item would close it as a side effect. The narrower window that remains genuinely unrecorded — a
   panic inside `NewApp` or `wails.Run`, before the trail exists — is
   [#62](https://github.com/robot-accomplice/dr-markdown/issues/62).
3. **Windows and Linux are unbuilt.** Only `tools/build-macos.sh` exists, and it builds
   `darwin/arm64` by default or `darwin/universal` with `--universal`. CI runs the full suite on Linux
   but produces no artifact. A release build matrix is real work — native runners, per-platform
   packaging — not a flag.
4. **The re-serialization class**, accepted since v0.4.0. 38 of 49 surveyed constructs round-trip
   byte-identically and **nothing is deleted any more**, but a construct nobody has surveyed is still
   respelled.

5. **Getting off Wails is now a roadmap item** — `wails-exit-path` in
   `docs/architext/data/roadmap.json`, priority medium, added 2026-08-10. Scoped from the source
   rather than from impression: Wails is imported by exactly two files, and every `runtime.*` call in
   `app.go` sits inside the `wailsNative` adapter, so **all 11 native operations are already behind
   `nativePort`** — the domain-ownership refactor bought most of a portability layer as a side
   effect. What is still coupled is `main.go`'s `wails.Run` bootstrap, the reflection binding, and
   `tools/build-macos.sh` delegating packaging to `wails build`. Two open items above are host
   limitations rather than app defects, and an exit closes both.

## In flight: ribbon presentation

`design/ribbon-presentation` carries an **approved design awaiting maintainer review**, at
`docs/superpowers/specs/2026-08-10-ribbon-presentation-design.md`. No plan written, no code touched.

Uniform-width ribbon buttons, five shortened Insert-tab labels, a never-truncate test gate, an
icons-only setting in Appearance, `Cmd+Shift+L`, and deriving the Settings shortcut list from the same
table the `keydown` handler dispatches from.

Two questions were put to the maintainer and are unanswered: whether the Insert tab should lose its
explicit labels (`Code block` → `Code`), and whether the shortcut-table change is wanted or just the
two lines that add one hotkey.

The View-menu item the maintainer asked for is **not** in that spec — see open item 1. There is no
View menu to add it to, though there is a menu bar to add one to.

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
