# M3.2 Raw Screen

## Objective

Implement the Raw mode screen from the concept handoff after the current formatted editor shell is functionally backed.

## Architectural Boundary

Raw mode is an alternate view of the active markdown buffer, not a separate document. It must preserve the same file rail, ribbon, title state, save/dirty behavior, and status bar contract. The raw editor surface owns source-oriented rendering and editor preferences that affect only Raw mode.

## Functional Surface

- Raw mode must switch from Formatted without losing markdown and must switch back without losing edits.
- The document area must show a source editor with line numbers, a mono gutter, and source-oriented spacing.
- The right panel must switch from Outline/Comments/Links to a Syntax Legend plus Raw editor toggles.
- Soft wrap and Line numbers must be visible controls with real behavior.
- The Raw status bar mode must show `RAW`.
- The Raw screen must not show unavailable Split, Preview, Share, Export, or fake workspace content.

## Deferred Subsystems

- CodeMirror-native markdown parsing is not available from the current vendored
  bundle; source highlighting is provided by the shared Highlight.js overlay
  added in M3.5.
- Hide markers remains deferred because hiding markdown sigils requires a true
  editable token/decoration layer rather than a visual highlighting overlay.
- User-persisted settings for Raw mode toggles belong to the Settings screen.
- Split mode has moved to the M3.3 screen. Scroll locking remains deferred until synchronization tests exist.

## Acceptance

- End-to-end tests cover Raw mode switching, line-number visibility, soft-wrap toggling, marker hiding, right-panel replacement, and markdown round-trip preservation.
- The screen continues to pass the current-shell no-decorative-controls test.
