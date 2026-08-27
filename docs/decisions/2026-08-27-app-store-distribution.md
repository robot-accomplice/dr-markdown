# Mac App Store distribution — deferred, with the blockers measured

Date: 2026-08-27. Status: **DEFERRED.** Not rejected, not scheduled. Recorded now because the
question arrives naturally with Developer ID enrolment, and the answer is not the one the pricing
page implies.

## The question

Enrolment in the Apple Developer Program costs $99 a year and covers both distribution paths. Does
notarizing the application for direct download also permit publishing it to the Mac App Store?

## The answer

**The membership covers it. The application does not.**

They are different paths rather than two depths of the same one:

| | Developer ID (direct download) | Mac App Store |
| --- | --- | --- |
| certificate | Developer ID Application | Apple Distribution + Mac Installer Distribution |
| artifact | `.dmg` | `.pkg` |
| notarization | required | not used; App Review instead |
| **App Sandbox** | **not required** | **mandatory** |

## Why the sandbox blocks this application specifically

Measured against the source at 1.6.3, not generalised from what sandboxing usually costs. Under the
sandbox, selecting a file through a picker grants access to **that file**, and to nothing else.

**1. It reads files the user never picked.** `assets.LoadForDocument` resolves a relative image path
against the document's own directory and reads it (`internal/assets/manager.go`, `os.ReadFile` at the
end of the containment checks). Opening `notes.md` grants `notes.md`; it does not grant
`notes.assets/figure.png` beside it. Every image in every document would fail to load, and the
failure would surface as the existing missing-asset render state, so it would look like data loss
rather than a permission refusal.

**2. It writes beside the document.** Image import creates `<document>.assets/` next to the file
(`os.MkdirAll` in the same package) and copies the asset in. That directory is not covered by the
grant either.

**3. It reopens paths from a stored list with no picker.** `App.OpenRecentDocument` opens a recorded
path directly, which is the point of a recents list. Under the sandbox that requires
**security-scoped bookmarks**: a bookmark created while access is held, persisted, resolved on use,
and released after. `internal/preferences` stores plain path strings and has no concept of a
bookmark.

## What adopting it would cost

Not a build flag. Asset resolution and the recents system would both be reworked around
security-scoped bookmarks, with the bookmark lifecycle threaded through the preference store and the
asset manager. It changes behaviour a user can see: a document whose assets folder was never
individually granted would need re-authorising, and a recents entry whose bookmark went stale would
need re-picking. Then App Review, on Apple's schedule, for every release.

## Why deferring is the right call today

The thing actually wanted is that **a stranger downloads the application and it opens**. Notarized
direct download delivers exactly that, for $99 and an afternoon, with no sandbox rework and no
review queue.

What the App Store adds beyond it is discovery and payments. This is an MIT-licensed editor with
neither a price nor a marketing surface, so both are worth close to nothing here.

## What would change the decision

- Wanting paid distribution, or App Store discovery, enough to fund the sandbox rework.
- Source-preserving editing landing first. The standing blocker — the WYSIWYG surface re-serializing
  the whole document — is the larger constraint on this application, and sandbox work ahead of it
  would be effort spent on reach rather than on correctness.
- Apple relaxing the sandbox requirement for the Mac App Store, which has not moved in years and
  should not be planned for.

## Related

- `#11` M12: packaging, signing, updates and platform hardening, which carries the notarization plan.
- `tools/build-macos.sh` gained opt-in Developer ID signing and notarization; the sandbox path has no
  equivalent and would not be a flag.
