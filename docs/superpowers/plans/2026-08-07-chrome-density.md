# M3.7 Chrome Density

## Objective

Remove redundant in-canvas application chrome and reclaim vertical space.

## Architectural Boundary

The native macOS window title owns the application name and current document
title. The embedded frontend shell should not repeat that title bar inside the
canvas. Application-level controls that still belong in the shell, such as
Settings, should share the ribbon tab row rather than occupying a separate
header.

## Functional Surface

- Remove the in-canvas title bar that repeats `Dr. Markdown`, the logo, document
  name, and save state.
- Keep native title updates through the Wails Go API.
- Move Settings into the ribbon tab row as an application-level control, not a
  View context command.
- Preserve mode controls and ribbon tab behavior.

## Acceptance

- End-to-end tests assert there is no rendered in-canvas title bar.
- End-to-end tests assert Settings is reachable from the ribbon tab row and not
  from the View ribbon panel.
- Full test suite remains green.
