# Responsive Shell Adaptation

GitHub issue: <https://github.com/robot-accomplice/dr-markdown/issues/14>

## Objective

Make the current shell degrade gracefully when the macOS window is reduced.
The app should feel like a native productivity application: controls compact,
panels preserve the document canvas, and primary actions do not become
partially clipped.

## Architecture

1. Keep the full ribbon labels at the default desktop width.
   - Verify the existing default visual baseline remains non-overflowing.
2. Collapse labelled ribbon controls at reduced widths.
   - Verify collapsed controls are icon-sized, still visible, and retain
     accessible names/tooltips.
3. Tighten side panels, contextual controls, and empty-state dimensions at
   reduced widths.
   - Verify the document canvas stays visible without horizontal page overflow.

## Test Plan

- Red first: e2e visual/layout test at default, medium, and narrow desktop
  widths.
- Green: CSS/markup changes only where needed for responsive behavior.
- Refresh README screenshots with `go run ./tools/screenshots` after visible UI
  changes.

## Acceptance

- Default width keeps text labels and fixed button spacing.
- Medium and narrow widths do not horizontally overflow the viewport.
- Labelled ribbon buttons collapse to icon-only where needed.
- Collapsed buttons retain `aria-label` and `title` tooltips.
- Export remains fully visible or intentionally compacted, never partially
  clipped.
