# M4.8 Print And PDF Export

## Objective

Provide backed print support and PDF export through the macOS print-to-PDF
pipeline described in the project design.

## Functional Surface

- Print and Export to PDF actions render the active markdown as a formatted
  print document, independent of the current Formatted/Raw/Split mode.
- Both actions invoke `window.print()` so macOS users can print or choose
  "Save as PDF" from the system print dialog.
- Print CSS excludes application chrome, side panels, contextual controls,
  dialogs, and status UI from the printed page.
- The print/export path does not mutate markdown or dirty state.

## Acceptance

- E2E verifies Print and Export to PDF call the print pipeline and render a
  formatted print document.
- E2E verifies raw/split modes print formatted content.
- E2E verifies print/export does not mutate markdown or dirty state.
