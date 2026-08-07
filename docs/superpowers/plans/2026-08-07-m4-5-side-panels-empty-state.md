# M4.5 Side Panels And Empty State Polish

## Objective

Tighten the current editor screen so the side panels and empty state feel like a
native document application rather than a browser layout.

## Functional Surface

- The empty state no longer repeats the app name or current document title that
  already live in the native window/title chrome.
- The empty state keeps backed document-start actions and templates, but removes
  instructional shortcut copy from the canvas.
- The document inspector is suppressed while the app is empty because there is
  no contextual document state to inspect.
- Minimized panels expose compact restore rails with clear accessible labels and
  stable collapsed widths.

## Acceptance

- E2E verifies the empty state avoids duplicated chrome text and hides the
  document inspector before a document exists.
- E2E verifies minimized side panels expose labelled restore controls, remain
  independently restorable, and do not mutate markdown.
