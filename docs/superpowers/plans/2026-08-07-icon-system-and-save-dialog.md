# M3.1 Icon System and Dialog Cleanup

## Objective

Replace hand-built CSS ribbon glyphs for insert actions with a reusable local icon system, and remove redundant custom alert iconography from save-related native dialogs where Wails exposes that control.

## Decisions

- Use inline SVG symbols instead of Font Awesome or Wingdings-style font glyphs. SVGs avoid another runtime dependency, render consistently on macOS, and remain inspectable in tests.
- Keep text labels beside the icon in ribbon buttons; icon-only actions are not the current ribbon pattern.
- Remove the redundant icon from Wails message dialogs by passing a tiny transparent icon. The native macOS save panel itself does not expose an icon option through Wails `SaveDialogOptions`.

## Verification

- Ribbon tests should assert the insert buttons use SVG icons instead of CSS pseudo-element drawings.
- Save/open failure and unsaved-change dialogs should share the same icon-free message dialog option.
- Full e2e suite must run after the diagram, icon, and dialog changes.
