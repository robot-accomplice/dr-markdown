# M7 Cursor-Precise Document Block Model

GitHub issue: <https://github.com/robot-accomplice/dr-markdown/issues/6>

## Objective

Replace first-detected contextual behavior with selected-block targeting. The
first implementation slice focuses on tables because table controls currently
mutate the first markdown table regardless of the table the user acted on.

## Architecture

1. Track the selected rendered block in frontend shell state.
   - Verify clicking a rendered table stores its table index.
   - Verify clicking a rendered Mermaid Diagram stores its fenced code index.
2. Route contextual table controls through the selected table index.
   - Verify row/column/alignment/delete commands mutate only the selected
     table when multiple tables exist.
3. Preserve existing code-block language targeting.
   - Verify right-click and hover language tools still target the acted-on code
     fence.
4. Route contextual Mermaid Diagram controls through the selected diagram fence.
   - Verify applying the assistant replaces the selected Mermaid fence rather
     than appending a new diagram.

## Test Plan

- Red first: e2e test with two rendered tables, click the second table, add a
  row, and assert only the second markdown table changes.
- Red first: e2e test with two rendered Mermaid diagrams, click the second
  diagram, apply the assistant, and assert only the second fence changes.
- Green: add block-selection metadata, indexed table rewrite helpers, and
  selected diagram fence rewriting.
- Broader verification: focused e2e, `go test ./... -count=1`, `architext
  validate .`, and screenshot refresh if visible UI changes.

## Acceptance

- Contextual table controls operate on the selected rendered table.
- Multiple tables can be edited independently from the document surface.
- Existing Mermaid Diagrams can be selected and replaced independently from the
  document surface.
- Existing code-block targeting behavior remains green.
