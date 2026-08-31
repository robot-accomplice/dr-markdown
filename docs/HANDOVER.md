# Handover — 2026-08-31 (rev 7)

State of play for whoever picks this up next. Written to be actionable without the conversation that
produced it.

Rev 7 replaces rev 6 wholesale. Rev 6 described the morning after the host replacement — Wails just
removed, the code-block blocker just fixed, VERSION still 0.5.1, PR #74 unmerged. All of that is four
releases old: 0.6.0, 1.6.1, 1.6.2 and 1.6.3 have shipped since. Nothing from rev 6 is still pending;
its lessons are kept below in shortened form, and its full text is in git history.

## Where the truth lives

- **Process:** `docs/RELEASE.md` is the release ceremony — written down after v1.6.3 was once recorded
  complete without ever being tagged or published. Every step runs, every release, in order.
- **Architecture and release facts:** `docs/architext/data/**`, validated by `architext validate`.
  Release records describe what shipped at the time; do not "modernize" their prose.
- **Go/no-go records:** `docs/releases/*-abort.md`. The 1.6.4 record is the one to read before
  skipping the ABORT step — that release was cut, reviewed, and withdrawn before anyone installed it.
- **Decisions:** `docs/decisions/`.

## Current state

**v1.6.3 shipped 2026-08-28** — "One surface, and a way to search it." Tagged, published, both DMG
assets (arm64 and universal), Release Truth complete. It carries the one-renderer fix for print and
split, document search and replace, the File menu, and the four blockers the 1.6.4 ABORT review found
(mermaid xlink navigation escaping the click guard, Cmd-Q bypassing the close guard, un-undoable
Replace All, frontend errors reaching nobody).

It was first cut as 1.6.4 and withdrawn before publication; the number went back because a version
nobody could install is not a version that happened.

`develop` is `main` plus the post-release morph-sweep test PR (#151). No open PRs, nothing in flight.
CI is green; the full local suite, e2e included, passes on this tree.

## Open threads

- **Issues:** only milestone trackers remain open — #5 (post-alpha roadmap), #10 (M10 export), #11
  (M12 packaging: update channels, checksums and packaged smoke tests remain; signing and
  notarization already ship), #12 (M9), #13 (M11). Every defect issue through #148 is closed.
- **Next release is not scoped.** Candidates, all still `planned` on the roadmap: the M12 remainder,
  the default-.md-handler offer (decision recorded 2026-08-25, one open question about macOS's own
  consent dialog), the `go-use-cases-out-of-app` refactor, and the M9/M10/M11 milestones. Scoping is
  a maintainer decision — do not write Release Truth for a release that has not been approved.
- **On-disk data directories keep the old name deliberately** (`dr-markdown`, no rebrand): renaming
  them would silently abandon settings, recents and the event trail on upgrade. The reason is
  recorded in the code and in the 1.6.3 release record.

## The rule that governs this editor

**WYSIWYG is the defining purpose.** Everything in Formatted mode must be editable in place; a
construct that renders but cannot be edited there is a defect, not a design choice — regardless of
what any document says. Recorded as a critical project rule in `docs/architext/data/rules.json`
(`wysiwyg-is-the-purpose`). This rule is why code blocks and mermaid diagrams became editable in
0.6.0, and it has been inferred away from documentation twice. Do not infer it away a third time.

## Lessons that keep costing rounds when forgotten

- **Every defect that mattered was found by a person using the app, not by a test.** The suite was
  green while the app had no copy, no paste, no ⌘Q, and uneditable code blocks. Drive the real
  application before believing green.
- **Presence assertions prove nothing.** Every code-block test checked that a label or button existed;
  none typed. The gates that catch real defects send keystrokes and require the characters to come
  back out of the document's markdown.
- **Prove the instrument before believing a negative.** Four harness failures in one session were the
  harness's own. A gate missing from the ceremony is a gate that does not exist.
- **Write the observation and the inference as separate sentences, and mark which one was driven.**
  Rev 5 published two true observations carrying false conclusions as measurements, and both pointed
  away from the cause. Reviewers check facts and pass; they do not re-derive conclusions.

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
  change, and `architext validate` must pass. It is not in CI — run it locally.
- The host gates (`-menu -close -quit -nav -walk -gates`, plus the dirty variants a human answers)
  run on the frozen release tree per `docs/RELEASE.md`. No Go test reaches them; they need a real
  AppKit application.
