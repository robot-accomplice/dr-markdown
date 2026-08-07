# M3.5 Syntax Highlighting

## Objective

Add backed syntax highlighting for markdown source views and language-tagged
fenced code blocks.

## Architectural Boundary

Syntax highlighting is a frontend rendering subsystem. The markdown buffer
remains plain text; highlighting must never mutate document content. Raw mode,
Split source mode, Split formatted preview, and formatted WYSIWYG code blocks
should all use one shared highlighter adapter so language aliases and fallback
behavior stay consistent.

## Dependency Boundary

The current vendored CodeMirror bundle exposes only `EditorView`, `basicSetup`,
and `minimalSetup`, so it cannot provide a markdown language/token layer. This
slice vendors a browser highlighter asset as an offline application asset and
wraps it in `frontend/dist/src/highlighter.js`.

## Functional Surface

- Raw mode displays markdown syntax highlighting while remaining editable.
- Split source displays markdown syntax highlighting while remaining editable.
- Fenced code blocks use the language declared after the opening backticks.
- Unsupported or missing code-block languages fall back to escaped plain code.
- Highlighting respects the code font and ligature settings.
- The highlighter is vendored with the app; no CDN or runtime network fetch is
  allowed.

## Deferred Subsystems

- Hiding markdown markers remains deferred until the source editor has a true
  token/decoration layer instead of a synchronized highlighted overlay.
- Per-code-block language selection from formatted mode is handled by the M3.8
  Code Block Language Assistant.
- Full all-language coverage can be expanded by changing the vendored
  highlighter build; this slice starts with the common-language browser build.

## Acceptance

- End-to-end tests cover raw markdown highlighting, split source markdown
  highlighting, and language-tagged fenced-code highlighting.
- Round-trip tests continue to prove highlighting does not mutate markdown.
- Vendor metadata and Architext Release Truth are updated before completion.
