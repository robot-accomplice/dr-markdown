# M3.8 Code Block Language Assistant

## Objective

Let users choose a code block language from the formatted/WYSIWYG workflow
without switching to Raw or Split mode.

## Architectural Boundary

The code block assistant is a frontend insert workflow. It inserts plain fenced
markdown into the active document using the selected language as the fence info
string. Syntax highlighting continues to derive from markdown through the
shared highlighter adapter.

## Functional Surface

- Visible Code Block controls open a language assistant instead of inserting a
  hardcoded `text` fence.
- The assistant provides a language selector for common programming, markup,
  data, shell, and plain-text languages.
- Confirm inserts a fenced code block with the selected language.
- Cancel closes the assistant without mutating markdown.
- The inserted block highlights in formatted mode using the selected language.

## Deferred Subsystems

- In-place language editing for an existing code block.
- Custom per-project language lists.
- A richer code block editor with filename/caption/copy controls.

## Acceptance

- End-to-end tests cover opening the assistant from the formatted Insert
  workflow, selecting a language, inserting a fenced block, immediate formatted
  highlighting, and canceling without document mutation.
- Existing internal command paths remain available for direct command tests.
