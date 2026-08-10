# Handover — 2026-08-10 (rev 5)

State of play for whoever picks this up next. Written to be actionable without the conversation that
produced it.

Rev 5 covers one long day with one large outcome and one blocked release: **Wails is gone from the
running application**, and **v0.6.0 could not ship** because code blocks were not editable.

> **Amended 2026-08-10 (rev 5.1).** The blocker is fixed by
> [#79](https://github.com/robot-accomplice/dr-markdown/pull/79). Two of the three "measurements"
> recorded below under the release blocker were **false**, and both pointed away from the cause —
> they are corrected in place rather than removed, because a later session trusting this file is the
> whole point of it. The proposed direction (write a ProseMirror NodeView) was also wrong and was not
> needed. See that section for what the cause actually was.

## The rule that governs everything below

**WYSIWYG is the defining purpose of this editor.** Maintainer, 2026-08-10:

> EVERYTHING should be editable in formatted mode, it's a WYSIWYG markdown editor

> WYSIWYG ... is the whole reason this app exists

Everything in Formatted mode must be editable in place. If a construct renders but cannot be edited
there, **that is a defect, not a design choice** — regardless of what any inventory, spec or prior
note says about that construct. Recorded as a critical project rule in
`docs/architext/data/rules.json` (`wysiwyg-is-the-purpose`) because the opposite had already been
inferred twice from documentation: the 2026-08-06 screen inventory promises block-local *language*
editing for rendered code blocks, and that was read as licence for the body to be read-only. It is
not.

## Start here

**The blocker is fixed.** [#77](https://github.com/robot-accomplice/dr-markdown/issues/77) is
resolved by [#79](https://github.com/robot-accomplice/dr-markdown/pull/79) on `fix/code-blocks-editable`,
which is open against `develop` with CI green. Start by merging that, then this PR (#74), then finish
the release ceremony below.

The ceremony was halted at 3 of 7 while the blocker stood, because finishing it would have meant
writing a Release Truth record asserting a readiness that was not true.

## The release blocker — FIXED, see [#79](https://github.com/robot-accomplice/dr-markdown/pull/79)

**Insert → Code produced a block you could not type into.**

> we cannot ship another broken release on basic editing functionality

This was **pre-existing** — it shipped in v0.5.1 and had since syntax highlighting was added. It was
not a consequence of the host replacement.

### What this section blamed, and why that was the symptom rather than the cause

`highlightFormattedCodeBlocks()` ran against `els.wysiwyg` — the LIVE editor — and for every code
block did two destructive things. That pass is now deleted, and it *was* wrong. But it was
compensation for a block that was already dead, not the reason the block was dead: the real cause is
below, and it is one undefined value in the vendored bundle.

```js
// app.js:1652
code.innerHTML = highlightCode(code.textContent, language)
wrapCodeBlock(code, language, fenceIndex)

// app.js:1753
function wrapCodeBlock(code, language = '', fenceIndex = null) {
  const pre = code.closest('pre')
  if (!pre || pre.closest('.code-block-shell')) return
  const source = code.textContent
  const shell = codeBlockElement(source, language, fenceIndex)
  pre.replaceWith(shell)      // the editor's node is thrown away
}
```

### RESOLVED 2026-08-10 by [#79](https://github.com/robot-accomplice/dr-markdown/pull/79). Two of these measurements were WRONG.

**This section recorded three measurements as fact. Two were false, and both pointed away from the
cause.** Corrected in place rather than deleted, because the whole value of this file is that a later
session trusts it, and because the error has a repeatable shape: a true observation carrying a false
conclusion.

1. **CORRECT — the vendored bundle is not missing anything.** `Crepe.Feature.CodeMirror` exists and
   is `"code-mirror"`; only `Latex` is disabled in `editor.js`.
2. **WRONG — "no CodeMirror is mounted anyway", concluded as "that feature never did this job".**
   The counts were right. The conclusion was not: it never mounts because it **throws**.
   `initializeCodeMirror()` raises `TypeError: Cannot read properties of undefined (reading
   'extension')` out of `EditorState.create`.
3. **WRONG — "a plain `<pre>` inside ProseMirror's contenteditable, so very likely editable
   already."** It is `pre.milkdown-code-block-placeholder` inside
   `div.milkdown-code-block[contenteditable="false"]`. It was never editable by anything, and the
   word "likely" is doing all the work in that sentence — it was never checked by typing.

**The cause.** esm.sh resolves the bare specifier `codemirror` inside the Crepe bundle to
**CodeMirror 5** — `defineMode`/`defineMIME`/`registerHelper`, no `basicSetup`, which is a v6 export.
Crepe builds `[keymap, fme.basicSetup, …]`, so entry two is `undefined`. `@milkdown/crepe` declares
`codemirror: ^6.0.1`, so this is an esm.sh bug, not a Crepe bug.

**Why it was permanent and silent.** The throw happens inside the `IntersectionObserver` callback
that upgrades a block, *after* the node view sets `initialized = true`. Nothing retries, and the
exception reaches nobody. A guard set before the work turns one crash into a permanent state.

**The fix was to drop that entry** in `tools/vendor.sh`. **No NodeView was needed** — the direction
below was wrong. The upstream node view already renders a searchable language picker, a copy button
and a preview toggle, and once it mounts it does the whole job, highlighting included. The app's
in-editor pass was deleted rather than rewritten, and mermaid moved onto the same node view's preview
hook, which made diagrams editable in place for the first time.

**Refuted, so nobody spends the day on them again:** supplying `extensions` via `featureConfigs` does
not displace the broken default (Crepe concatenates); the separately vendored `codemirror.bundle.mjs`
cannot supply it either (second copy of `@codemirror/state`, rejected outright) and has been deleted;
and the fetch cannot be fixed with `?deps=`/`?alias=` because the es2022 path is prebuilt.

**The lesson this cost.** Every code-block test asserted *presence* — a label, a Copy button, a
highlighted span — and every one passed for the entire time the block underneath was inert, because
**none of them ever typed**. The gate that finally caught it sends real keystrokes and requires them
in the document's markdown.

## What merged into `develop` today

| PR | |
| --- | --- |
| [#70](https://github.com/robot-accomplice/dr-markdown/pull/70) | `VERSION` is the single source of build identity; drift gate proven able to fail |
| [#71](https://github.com/robot-accomplice/dr-markdown/pull/71) | empty state no longer clipped off the top by a long recents list |
| [#72](https://github.com/robot-accomplice/dr-markdown/pull/72) | Paste survives a clipboard the platform refuses |
| [#73](https://github.com/robot-accomplice/dr-markdown/pull/73) | the release gate and the elimination inventory |

## The open PR: the host replacement

**[#74](https://github.com/robot-accomplice/dr-markdown/pull/74) `refactor/own-the-host-cutover`,
CLEAN, 5 commits ahead of `develop`.** Complete and verified; unmerged because the release it belongs
to is blocked.

**Wails is gone from the running application.** `go.mod` carries chromedp (tests only),
`golang.org/x/image` and `golang.org/x/sys`. Zero `wailsapp` modules. Binary 12.3MB → 10.4MB.
Everything the application asks of the OS is answered by AppKit and WebKit directly, behind
`hostPort`. **`app.go` was not touched by the swap.**

### Why it had to be one commit

`bridge.js` degrades when the host is **absent** but throws when it is **partial** — several entries
are written `native()?.Method(x)` rather than `native()?.Method?.(x)`, so once the object exists a
missing method is a TypeError and boot dies. There was never a staged path, and rev 4's "second
adapter alongside Wails" plan was wrong for that reason.

### Host defects found and fixed on the way

Each real, none would have failed a test:

- **`DefaultButton` was dropped in the port.** Return on the overwrite prompt destroyed the file.
- **`dispatch_async` to the main queue is not serviced during a modal**, so any open dialog froze
  every pending reply — and the dialog is usually the report of the failure the caller awaits.
- **`dispatch_sync` held a cgo call open for the life of a dialog**, pinning an OS thread and making
  the `context.Context` every `nativePort` method takes impossible to honour. Now answered over a
  channel, with `orDone`/`or` giving every goroutine an exit.
- **There was no menu bar at all.** A Cocoa app gets none unless it builds one, and the Edit menu's
  key equivalents are what deliver ⌘C, ⌘V, ⌘X, ⌘A and ⌘Q. Measured `mainMenu=NIL`. Building it also
  closes [#57](https://github.com/robot-accomplice/dr-markdown/issues/57).
- **Three shortcuts were stolen by that new menu** — ⌘B (bold), ⇧⌘S (split), ⌘W (close tab, which the
  menu had closing the window). A menu key equivalent beats the webview, so all three would have
  shipped silently.

### Verification, and its honest limit

`go run . -gates -walk -menu -close -close-dirty -drop -doc <file> -modal N`

Gates: the frontend boots, a bound call round-trips, **a panicking bound call REJECTS rather than
hanging** ([#61](https://github.com/robot-accomplice/dr-markdown/issues/61)), events reach the
frontend, a real mouse drag arrives, and a document round-trips to disk byte-exactly. The walk is 40
checks across the 2026-08-06 screen inventory. All pass, including from inside the `.app`.

**None of it runs in CI.** It drives a native window, chromedp cannot see it, and there is no macOS
runner. Every *Go* test is untagged and runs on Linux; the host gates are a manual step on a Mac.

## Packaging works

`tools/build-macos.sh` replaces `wails build`, which no longer exists in this tree: compile, bundle,
`Info.plist` from `VERSION`, `.icns` from `build/appicon.png`, ad-hoc sign, DMG. `--universal`
verified — `lipo -archs` reports `x86_64 arm64`. `wails.json` is deleted.

## Documentation is current

Architext no longer describes a Wails application. Node ids renamed (`wails-desktop-app` →
`desktop-app`, `wails-bridge` → `native-bridge`, `wails-go-api` → `go-api`) across every referencing
file; historical release records keep their prose because they record what was true then. The
`own-the-host` decision carries the three objections that drove it and the fourth that was **checked
and refuted** — both `net.Listen` sites in Wails v3 sit behind the `server` and `mcp` build tags, so
the TCP claim should not be derived a third time. `no-recorded-state-for-rca` is closed. README
rewritten, screenshots regenerated, `architext validate` passes.

## The remaining ceremony, deliberately not done

1. **Release Truth record + `VERSION` bump** — `VERSION` still reads `0.5.1`.
2. **Close resolved issues.** [#61](https://github.com/robot-accomplice/dr-markdown/issues/61) and
   [#57](https://github.com/robot-accomplice/dr-markdown/issues/57) are genuinely fixed and should
   close **when #74 merges**, not before.
3. **ABORT stations.**
4. **roboticus-site** project card — `src/lib/projects-data.ts` carries `currentVersion`.
5. Rebuild from the merge commit, install the DMG, open a document before publishing.

## Issues opened today

- [#77](https://github.com/robot-accomplice/dr-markdown/issues/77) — code blocks not editable. **Was
  the blocker; fixed by [#79](https://github.com/robot-accomplice/dr-markdown/pull/79).**
- [#75](https://github.com/robot-accomplice/dr-markdown/issues/75) — the block-edit `+` menu appears to
  do nothing. Guessed at the time to be the same cause as #77 — "it inserts a block that renders as
  an empty box you cannot type into". **Recheck against #79 rather than closing on that guess**: an
  inserted code block is now editable immediately, so if the `+` menu still does nothing the cause is
  something else and the guess was never evidence.
- [#78](https://github.com/robot-accomplice/dr-markdown/issues/78) — opened 2026-08-10 with #79: the
  editor's own language picker writes a language's DISPLAY name into the fence (` ```Python `), where
  every other route in this app writes ` ```python `.
- [#76](https://github.com/robot-accomplice/dr-markdown/issues/76) — block-edit menu highlight colour
  is off-theme. Note an app rule on an editor-injected element loses a specificity tie to
  `.milkdown *`; scope to (0,2,0).

**Not a regression, checked:** ribbon buttons have never had uniform widths. The only CSS changes
since `v0.5.1` are the `#empty-state` line and PR #67's code-block-header scoping, neither in ribbon
rules. `design/ribbon-presentation` is 148 lines of spec and no code, still waiting on two maintainer
answers.

## The lesson from this session

**Every defect that mattered today was found by a person using the app, not by a test.** The suite was
green throughout: while the application had no copy, no paste and no ⌘Q; while the empty state was
clipped off the top of the screen; while Paste died on a denied clipboard; and while code blocks
could not be edited.

The corollary is about instruments. Four times a harness reported a failure that was the harness's
own: a raw newline in an Objective-C string literal killed the whole injected script (and the app
still looked healthy, because `bridge.js` degrades); an instruction window took key focus from the
modal it was describing; a backtick in a Go comment terminated a raw string, so a stale binary
reported the previous run's results; and a walk that raced `boot()` reported 39 failures as
application defects. **Prove the instrument before believing a negative** is already in this
project's lessons. It cost four rounds anyway.

## Ground rules that are enforced, not aspirational

- **Modified gitflow**: branch → PR to `develop` → PR to `main`. Branch protection requires the
  `test` check and an up-to-date branch. Expect a PR to sit `BEHIND` after another merges; update it.
- Only **robot-accomplice** credentials: `export GH_TOKEN=$(gh auth token --user robot-accomplice)`.
- **No Claude/Anthropic attribution** in commits, PRs or artifacts.
- **No Node toolchain.** `node --check` is a syntax gate only.
- **Take the local gate FROM CI, never from memory.** Today:
  `gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1`.
- **A Go raw string cannot contain a backtick**, and Objective-C compiles `\n` in a literal into a
  real newline. Both have silently broken injected JavaScript here. Serve large scripts through the
  scheme handler instead.
- Architext data under `docs/architext/data/**` must be updated when architecture or trust boundaries
  change, and `architext validate` must pass.
