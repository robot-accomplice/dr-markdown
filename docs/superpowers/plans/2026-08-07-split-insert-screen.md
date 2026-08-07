# M3.3 Split And Insert Screen

## Objective

Implement the next concept screen after Raw mode: Split view plus the Insert popover.

## Architectural Boundary

Split mode remains one active markdown buffer. The source side edits markdown, the formatted side is a derived preview of the same markdown. Split must preserve existing file rail, title/dirty state, ribbon command routing, save behavior, and status bar state.

## Functional Surface

- A visible `Split` control may return only when it switches to a real split surface.
- Split mode must show two equal panes with headers: `Source` and `Formatted`.
- The source pane must edit markdown and update the active document.
- The formatted pane must render a derived preview from the same markdown without pretending to be a full second WYSIWYG editor.
- Switching Formatted, Raw, and Split must preserve markdown.
- The Insert tab/group must expose a backed popover menu, not only static ribbon buttons.
- Insert popover items must use existing markdown command paths or stay absent.
- Mermaid Diagram insertion opens the diagram-type assistant before mutating
  markdown.

## Deferred Subsystems

- Scroll locking is deferred until both panes have stable scroll synchronization tests.
- Full formatted rendering parity with Milkdown is deferred; the first split preview can cover headings, paragraphs, lists, quotes, highlighted code blocks, tables, links, mermaid placeholders, and horizontal rules.
- Image, math, footnote, export, preview, and share remain deferred until their backing subsystems exist.

## Acceptance

- End-to-end tests cover entering Split, editing source, preview refresh, mode round-trip preservation, Insert popover open/close, and popover item command execution.
- The no-decorative-control invariant still passes.
- Architext Release Truth is updated before completion.
