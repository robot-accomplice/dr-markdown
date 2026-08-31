# Offering to become the default `.md` application — design

Date: 2026-08-25. Status: approved, not implemented. Revised 2026-08-31: the open
question below is answered by experiment, and the answer changed the design — the
first-launch prompt is gone, replaced by a menu item.

## What was asked

> during install Dr. Markdown should offer to associate itself with .md files as the default
> application

## The constraint that reshapes it

**There is no installer.** `tools/build-macos.sh` produces a `.app` and a DMG containing it beside a
symlink to `/Applications`; the user drags it across. There is no install step to hook, so "during
install" has to become something else. First launch was the original answer; the 2026-08-31
experiment moved it again, to a **standing menu item** — see the decisions table.

What already exists: the bundle declares `CFBundleDocumentTypes` and `UTImportedTypeDeclarations`
for `md` and `markdown`, so Dr. Markdown appears under **Open With** today. What it never does is
*become the default*. No code in the repository calls Launch Services.

## Decisions

| Question | Answer |
| --- | --- |
| When to ask | **Never unprompted** (revised 2026-08-31 — see the answered open question). Originally "once, on first launch"; the OS now asks for us, so a prompt of our own would be a second interruption. The offer is a menu item the user can choose at any time. |
| What to claim | **The UTI `net.daringfireball.markdown`**, which covers `.md` and `.markdown` together — exactly what the bundle already advertises |
| Where the offer lives | A **native menu item** in the menu bar the host builds, behind the existing `hostPort` |
| API | **`LSSetDefaultRoleHandlerForContentType`** |

### Why a menu item rather than any prompt

Originally the offer was a native alert on first launch, chosen over a webview modal because a
webview dialog asking to change a *system* setting reads as less trustworthy than the OS's own
furniture. The 2026-08-31 experiment (below) showed macOS presents its own consent dialog for the
Launch Services call itself — so any prompt of ours, native or webview, would be the second of two
interruptions about the same question. What remains is a standing, unintrusive offer: a menu item.

### Why that API

The deployment target is **macOS 11.0** (`LSMinimumSystemVersion` in `build/darwin/Info.plist`).
`NSWorkspace.setDefaultApplication(at:toOpen:)` is 12.0+, so it would need a version gate *plus*
this call as the fallback anyway. `LSSetDefaultRoleHandlerForContentType` works across every
supported version: one code path, no branch that only executes on machines nobody tests on.

It is deprecated as of 12.0 and still functional. If the floor ever rises to 12, switch — and say so
in a comment at the call site rather than leaving the deprecation to be rediscovered.

## Shape

Two methods on `hostPort`, mirroring how every other native operation is bound:

- `IsDefaultMarkdownHandler(ctx) (bool, error)` — `LSCopyDefaultRoleHandlerForContentType`, compared
  against our own bundle identifier
- `SetDefaultMarkdownHandler(ctx) error` — `LSSetDefaultRoleHandlerForContentType` for
  `kLSRolesEditor | kLSRolesViewer`

The menu item's state is **derived, never assumed**: menu validation re-queries
`IsDefaultMarkdownHandler`, because `Set…` returns success before the user has answered the system
dialog (measured 2026-08-31 — see below). When we are already the default, the item is checked and
disabled.

## Two guards, because the offer is wrong in two situations

1. **Already the default.** The menu item is checked and disabled; there is nothing to offer.
2. **Running from a mounted disk image.** The item is disabled. Never set from there.

The second guard is the one that matters most and is easiest to miss. People routinely run an app
straight out of the DMG before dragging it across. Setting the default handler to a bundle under
`/Volumes/…` points every future `.md` double-click at a volume that will be unmounted — turning a
helpful offer into a broken file association that the user then has to diagnose. Refuse there.

The guard is on the bundle path being on a mounted image, not on being under `/Applications`
specifically: people legitimately keep applications in `~/Applications` and elsewhere, and a check
that insists on one location would refuse a correct install.

The original design had a third guard — "already answered, never ask again", persisted on
`Preferences`. It is deleted: with no prompt of our own there is nothing to throttle, and the OS's
dialog is the consent. No preference is added.

## Testing

The split is deliberate, because only half of this can run in CI.

**The decision is pure and gets ordinary Go unit tests.** *What should the menu item show?* is a
function of (is-default, bundle-path) and both guards are testable without touching Launch Services
or drawing anything.

**The Launch Services calls sit behind the port** and join the manual host gates
(`go run . -gates …`). They cannot run in CI: chromedp cannot see a native window, and while
`ci.yml`'s `darwin` job now builds, tests and packages the app, it does not drive one.

## Open question — ANSWERED 2026-08-31 by driving the built app

**Whether recent macOS presents its own consent dialog when an application changes a default
handler.** It does. Measured on macOS 26.6.2, calling `LSSetDefaultRoleHandlerForContentType` from a
bare unsigned harness binary (less trusted than the real app will ever be):

1. macOS presented its own dialog offering the current default and the new application by name, and
   **the change did not apply until the user answered** — a `get` immediately after a successful
   `set` still showed the old handler.
2. `LSSetDefaultRoleHandlerForContentType` **returned status 0 immediately**, while the change was
   still pending consent. The call's result carries no information about the outcome; state must be
   re-queried.

Two consequences, both now folded into the design above: our own first-launch prompt is dropped (the
OS asks), and the menu item derives its state from a fresh query rather than from the call's return
value.

## Not in scope

- Windows and Linux. macOS is the only packaged platform.
- Reclaiming the association if another application takes it later. Nothing proactive: because the
  menu item's state is derived on every menu validation, another application taking the default
  simply re-enables the item. No notification, no watcher.
- A Settings toggle. Considered and not chosen: it is close to invisible, since most users never
  open Settings, and the menu item already covers the moment the question is live without burying
  it in a modal.
