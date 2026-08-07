# M4.9 Visual Regression Baseline

## Objective

Create an automated visual baseline guard for the current milestone screens so
future layout work catches blank renders, ribbon overflow, and major responsive
breakage early.

## Functional Surface

- Screenshot coverage exercises the empty, formatted, raw, split, and settings
  surfaces at a constrained desktop window size.
- Each captured state must render nonblank pixels and stay within the viewport
  without horizontal page overflow.
- The Home ribbon's Export control must remain fully visible at the constrained
  default review size.

## Acceptance

- E2E screenshot coverage verifies all baseline states render nonblank.
- E2E verifies document width does not overflow the viewport.
- E2E verifies the Export control remains visible in the default ribbon layout.
