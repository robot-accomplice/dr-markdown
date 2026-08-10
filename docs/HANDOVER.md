# Handover — 2026-08-10 (rev 5)

State of play for whoever picks this up next. Written to be actionable without the conversation that
produced it.

Rev 5 covers one long day with one large outcome and one blocked release: **Wails is gone from the
running application**, and **v0.6.0 cannot ship** because code blocks are not editable.

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

**Read [#77](https://github.com/robot-accomplice/dr-markdown/issues/77) and start there.** Everything
else on this page is either finished or waiting on it.

The release ceremony is deliberately halted at 3 of 7. Finishing it would mean writing a Release
Truth record asserting a readiness that is not true.

## The release blocker

**Insert → Code produces a block you cannot type into.**

> we cannot ship another broken release on basic editing functionality

This is **pre-existing** — it ships in v0.5.1 today and has since syntax highlighting was added. It is
not a consequence of the host replacement.

### Root cause, measured

`highlightFormattedCodeBlocks()` runs against `els.wysiwyg` — the LIVE editor — and for every code
block does two destructive things:

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

### Three measurements, each of which killed a proposed fix

Recorded because each was believed before it was checked:

1. **The vendored bundle is not missing anything.** `Crepe.Feature.CodeMirror` exists and is
   `"code-mirror"`; only `Latex` is disabled in `editor.js`.
2. **No CodeMirror is mounted anyway.** With the app's `replaceWith` suppressed: `cm-editor: 0`,
   `cm-content: 0`, `pre: 1`. So editor syntax highlighting never came from that feature — it comes
   from the app calling Highlight.js and rewriting `innerHTML`.
3. **The native node is a plain `<pre>` inside `div.ProseMirror[contenteditable]`**, so it is very
   likely editable already, just unhighlighted.

### Why both obvious fixes are wrong

- **Stop decorating inside the editor** (chrome only on preview/print) — rejected by the maintainer:
  it makes Formatted mode stop looking like the output, which violates the rule at the top of this
  page.
- **Wrap instead of replace** — also wrong, and this is the non-obvious part. Both operations mutate
  ProseMirror's DOM from outside, and ProseMirror reconciles that DOM. Replacing is almost certainly
  what the original author landed on *because* it is the only way to make an external mutation stick.

### The direction

A **ProseMirror NodeView for `code_block`** that owns the whole block: the header (language + Copy),
the editable content ProseMirror manages, and highlighting applied inside its own render rather than
by overwriting the document. Chrome visible AND editable, one system owning the DOM.

**Check first, because it may be much cheaper:** why `code-mirror` mounts nothing. If that feature
can be made to work it provides editing and highlighting together, and the app's job shrinks to
attaching a header.

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

- [#77](https://github.com/robot-accomplice/dr-markdown/issues/77) — code blocks not editable. **The blocker.**
- [#75](https://github.com/robot-accomplice/dr-markdown/issues/75) — the block-edit `+` menu appears to
  do nothing. Probably the same cause as #77: it inserts a block that renders as an empty box you
  cannot type into.
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
