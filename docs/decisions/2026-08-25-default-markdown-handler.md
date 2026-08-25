# Offering to become the default `.md` application — design

Date: 2026-08-25. Status: approved, not implemented.

## What was asked

> during install Dr. Markdown should offer to associate itself with .md files as the default
> application

## The constraint that reshapes it

**There is no installer.** `tools/build-macos.sh` produces a `.app` and a DMG containing it beside a
symlink to `/Applications`; the user drags it across. There is no install step to hook, so "during
install" has to become something else, and **first launch** is the closest honest equivalent —
approved.

What already exists: the bundle declares `CFBundleDocumentTypes` and `UTImportedTypeDeclarations`
for `md` and `markdown`, so Dr. Markdown appears under **Open With** today. What it never does is
*become the default*. No code in the repository calls Launch Services.

## Decisions

| Question | Answer |
| --- | --- |
| When to ask | **Once, on first launch.** The answer is stored either way and never asked again. |
| What to claim | **The UTI `net.daringfireball.markdown`**, which covers `.md` and `.markdown` together — exactly what the bundle already advertises |
| Where the prompt lives | A **native alert**, behind the existing `hostPort` |
| API | **`LSSetDefaultRoleHandlerForContentType`** |

### Why a native alert rather than a webview modal

The app has its own modals, but a webview dialog asking to change a *system* setting reads as less
trustworthy than the OS's own furniture, and it would need a bridge round-trip purely for
presentation. The host already builds native dialogs, including the `DefaultButton` handling that
#74 fixed after a dropped default let Return destroy a file.

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

## Three guards, because the offer is wrong in three situations

1. **Already the default.** Never ask.
2. **Already answered.** Never ask again, whichever way it was answered. Persisted as a typed field
   on `Preferences`, not a loose key in the `Settings` map — this is application lifecycle state, not
   a user-facing setting, and the settings map is rendered into the Settings modal.
3. **Running from a mounted disk image.** Never ask, and never set.

The third guard is the one that matters most and is easiest to miss. People routinely run an app
straight out of the DMG before dragging it across. Setting the default handler to a bundle under
`/Volumes/…` points every future `.md` double-click at a volume that will be unmounted — turning a
helpful prompt into a broken file association that the user then has to diagnose. Refuse there.

The guard is on the bundle path being on a mounted image, not on being under `/Applications`
specifically: people legitimately keep applications in `~/Applications` and elsewhere, and a check
that insists on one location would refuse a correct install.

## Testing

The split is deliberate, because only half of this can run in CI.

**The decision is pure and gets ordinary Go unit tests.** *Should we offer?* is a function of
(is-default, has-answered, bundle-path) and all three guards are testable without touching Launch
Services or drawing anything.

**The Launch Services call sits behind the port** and joins the manual host gates
(`go run . -gates …`). It cannot run in CI: chromedp cannot see a native window, and while
`ci.yml`'s `darwin` job now builds, tests and packages the app, it does not drive one.

## Open question, to be answered by driving the built app

**Whether recent macOS presents its own consent dialog when an application changes a default
handler.** If it does, the user meets two prompts — ours and the system's — and ours should probably
go, leaving the offer as a menu item rather than a launch-time interruption.

This is deliberately recorded as unresolved. It is a question about what the OS does, and this
project's own most expensive lesson is that when the answer depends on a framework, an OS or a
browser, your own source is not admissible evidence. Check it by running the app before the first
line of the prompt is written.

## Not in scope

- Windows and Linux. macOS is the only packaged platform.
- Reclaiming the association if another application takes it later. Asking once means asking once.
- A Settings toggle. Considered and not chosen: it is close to invisible, since most users never
  open Settings, and the first-launch offer already covers the moment the question is live. If
  "Not now" turns out to need an undo, a menu item is the cheaper answer than a settings row.
