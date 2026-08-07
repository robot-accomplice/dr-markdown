# M4.6 Contextual Document Controls

## Objective

Move context-sensitive block management onto the document surface instead of
forcing users back to the ribbon for table, code block, and diagram work.

## Functional Surface

- A contextual document toolbar appears only when the active markdown contains
  supported block types.
- Table controls add/remove rows and columns, adjust alignment, and delete the
  first detected table through the same markdown transforms used by commands.
- Code block controls allow language selection for the first non-Mermaid fenced
  code block without switching to raw or split mode.
- Mermaid Diagram controls open the guided diagram assistant from the document
  surface.

## Acceptance

- E2E verifies contextual controls are absent for an empty document and present
  for table/code/diagram content.
- E2E verifies table and code language controls mutate markdown and refresh the
  formatted document surface.
- E2E verifies Mermaid Diagram assistance is reachable from the contextual
  control.
