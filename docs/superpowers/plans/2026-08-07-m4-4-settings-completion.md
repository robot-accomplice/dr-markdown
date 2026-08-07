# M4.4 Settings Completion

## Objective

Make the settings surface match the design package without pretending future
sections are backed.

## Functional Surface

- Settings lives in the primary tab row and is not treated as a View context.
- Backed Editor settings control default editor mode, formatted syntax markers,
  editor width, soft wrap, line numbers, and format-on-save behavior.
- Appearance settings continue to control theme, document font, document size,
  code font, and ligatures.
- Markdown flavour, Sync & Git, and Extensions are visible as disabled future
  categories until native behavior exists.
- Runtime settings apply immediately after Save and Cancel discards drafts.

## Acceptance

- E2E verifies runtime preferences apply to the app surface.
- E2E verifies disabled future sections are present but unavailable.
- E2E verifies Editor design controls are backed and affect new-document mode,
  CSS editor width, formatted markers, and format-on-save state.
