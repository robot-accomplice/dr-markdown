# M4.2 Split Mode Parity

## Objective

Make Split mode match the design's paired source/rendered workflow: equal panes,
explicit scroll-lock labeling, and synchronized scrolling.

## Architectural Boundary

Split scroll synchronization is frontend shell behavior. It coordinates two
views of the same markdown buffer and must not mutate markdown content, dirty
state, or native file state.

## Functional Surface

- The formatted pane header reads `Formatted · scroll locked`.
- Split panes are height-constrained to the workspace instead of expanding with
  document content.
- Scrolling source moves formatted preview proportionally.
- Scrolling formatted preview moves source proportionally.
- Source syntax overlay continues to track textarea scroll.

## Acceptance

- E2E covers the scroll-locked header and both scroll directions.
- Existing split edit/round-trip tests continue to pass.
