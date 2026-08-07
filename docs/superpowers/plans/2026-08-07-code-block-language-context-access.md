# Code Block Language Context Access

## Objective

Let users edit the language for an existing code block directly from that block,
without switching to Raw or Split mode and without relying on the global
contextual toolbar's first-code-block behavior.

## Functional Surface

- Non-Mermaid code blocks expose a hover tool for language editing.
- Right-clicking a non-Mermaid code block opens the same language dialog while
  keeping the browser context menu suppressed.
- The dialog targets the acted-on fenced block, so multi-code-block documents do
  not accidentally edit the first block.
- Applying a language change rewrites only the targeted fence info string,
  remounts the formatted surface, and preserves the code body.

## Acceptance

- E2E verifies right-click language editing updates the second code block in a
  two-code-block document.
- E2E verifies the hover language tool opens the same dialog and updates the
  first code block.
- E2E verifies browser context-menu default behavior remains suppressed.
