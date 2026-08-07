# M4.3 Ribbon Completion And Command Taxonomy

## Objective

Move the ribbon closer to the design package's command maturity while preserving
the backed-controls rule.

## Functional Surface

- Home/Insert expose backed Link, Image, Table, Code, Math, and Mermaid Diagram
  insertion commands.
- Help is a real tab with backed Markdown Help and Keyboard Shortcuts panels.
- Export opens a backed menu containing Print and Export to PDF actions for the
  later print/PDF implementation slice.
- Share remains hidden until collaboration behavior exists.

## Acceptance

- E2E verifies the expanded Insert command set is present and mutates markdown
  for Image and Math.
- E2E verifies Help opens a backed panel.
- E2E verifies Export opens a backed menu with Print and PDF actions.
- Existing decorative-control and ribbon-fit tests pass.
