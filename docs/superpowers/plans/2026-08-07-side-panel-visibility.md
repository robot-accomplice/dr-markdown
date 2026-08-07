# M3.1 Side Panel Visibility

## Objective

Allow the workspace file rail and right document panel to be minimized or hidden independently without changing the active document, editor mode, or markdown content.

## Source-backed Design

- The left rail owns workspace file navigation and new-document access.
- The right panel owns outline/comments/links in formatted and split modes, and raw-editor assistance in raw mode.
- Each panel needs its own toggle so users can reclaim canvas width without entering global Focus Mode.

## Verification

- E2E coverage should hide and restore the left and right panels independently.
- Hiding one panel must not hide the other panel or mutate markdown.
- Collapsed panel state must survive mode switches because the app rebuilds the right panel when switching Raw/Formatted/Split.
