# M4.7 Accessibility And Native Feel Pass

## Objective

Make the current screen's transient surfaces and mode controls behave like a
desktop document application with predictable keyboard and accessibility state.

## Functional Surface

- Escape closes export/help/settings/code/diagram transient UI without mutating
  markdown.
- Settings, code, and diagram dialogs move focus to their first actionable
  control when opened.
- Formatted, Raw, and Split mode controls expose `aria-pressed` state that stays
  synchronized with the editor mode.
- Existing browser-default suppression remains in place for context menus and
  drag/drop.

## Acceptance

- E2E verifies Escape dismissal across transient surfaces.
- E2E verifies dialog focus landing.
- E2E verifies mode controls expose accurate pressed state after mode changes.
