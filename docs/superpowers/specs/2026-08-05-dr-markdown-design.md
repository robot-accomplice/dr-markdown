# Dr. Markdown — Design Spec

**Date:** 2026-08-05
**Status:** Approved (design phase)
**License:** MIT (long-term goal: open-source Typora killer)

## Vision

A native, cross-platform (macOS, Windows, Linux) WYSIWYG markdown editor with
Microsoft Word-like richness, built on markdown as the single source of truth.
First-class support for GFM, visual tables, images, syntax-highlighted code
blocks, and inline-rendered mermaid diagrams. Users can toggle instantly
between WYSIWYG and raw markdown editing.

## Hard Constraints

- **No Node.js tooling or dependencies whatsoever** — not at dev time, build
  time, or runtime. The entire build is `go build`.
- Third-party JS (editor engine, mermaid) is vendored as pre-built single-file
  ESM bundles, committed to the repo, and embedded via `go:embed`.
- Platforms: macOS, Windows, Linux from one codebase.

## Key Decisions (from brainstorming)

| Decision | Choice | Rationale |
|---|---|---|
| "Native" meaning | Native shell (Wails) + OS webview | Only realistic path to Word-like richness; how Typora/Obsidian-class apps work |
| Language | Go + Wails (latest stable release; exact version pinned at implementation start) | Pure-Go toolchain, mature, simple cross-platform builds |
| Editor engine | Milkdown (ProseMirror-based) + CodeMirror 6 (raw mode) | Battle-tested markdown round-tripping; escape hatch to raw ProseMirror later |
| App shape | Single-document + tabs | Focused scope; workspace features deferred |
| Dialect | GFM (CommonMark + tables, strikethrough, task lists, autolinks) + mermaid | Pragmatic standard, round-trips cleanly |
| Rich elements | Visual tables, images, code highlighting, mermaid editing UX | All first-class |
| Export | HTML + PDF (print pipeline); DOCX deferred to later milestone | DOCX without pandoc/Node means writing a Go docx generator |

Rejected alternatives: raw ProseMirror custom schema (2–3x effort, keep as
future option since Milkdown is ProseMirror), TOAST UI Editor (low
customization ceiling, poor Typora-killer foundation), pure native widgets
egui/Fyne (multi-year effort for a custom rich-text engine).

## Architecture

Three layers, strictly separated:

1. **Go shell (Wails)** — window, native menus, file dialogs, file I/O, tab
   dirty-state tracking, image asset copying, export file writing. Exposes a
   small typed API to the frontend via Wails bindings. Knows nothing about
   ProseMirror or markdown semantics.
2. **Vendored frontend assets** (`frontend/vendor/`) — single-file ESM bundles
   of Milkdown+ProseMirror, CodeMirror 6, mermaid.js. Fetched once via
   `frontend/tools/vendor.sh` (curl, pinned versions), committed, embedded
   with `go:embed`. Licenses recorded in `frontend/vendor/NOTICE`.
3. **Application JS** (`frontend/src/`, hand-written ESM, no build step) —
   ribbon UI, tab management, mode toggle, mermaid node view, Wails glue.

Either outer layer can be replaced without touching the other.

### Repository Layout

```
dr-markdown-md/
  main.go                  # Wails entry
  internal/
    app/                   # lifecycle, Wails bindings
    document/              # open/save/tabs/dirty state, atomic save
    images/                # asset copy + relative path resolution
    exporter/              # HTML/PDF file writing
  frontend/
    index.html
    src/                   # hand-written ESM: ribbon, tabs, modes, mermaid
    vendor/                # milkdown, codemirror, mermaid (committed bundles)
    tools/vendor.sh        # fetch+pin script
  docs/superpowers/specs/
```

## Data Flow & Core Contracts

### Round-trip contract

Markdown text is canonical. Everything the ribbon inserts must serialize to
clean, predictable GFM.

- **Load:** markdown string → Milkdown parse (remark, GFM preset) →
  ProseMirror document.
- **Edit:** user edits PM document; debounced serialize back to markdown for
  dirty-checking (text comparison against last-saved, not AST diffing).
- **Save:** serialize PM doc → markdown → Go writes bytes. The on-disk file is
  always exactly what raw mode shows.

### Mode toggle

- To raw: serialize PM doc → load into CodeMirror 6 (markdown highlighting,
  line numbers, no ribbon).
- To WYSIWYG: take CodeMirror text → parse → rebuild PM doc.
- CommonMark parsing is total (cannot fail on any input).
- **Opaque-block rule:** anything outside the supported dialect (YAML front
  matter, raw HTML blocks, unknown constructs) renders in WYSIWYG as a
  read-only opaque block and round-trips byte-identically. Never silently drop
  or rewrite.

### Mermaid

- A fenced code block with language `mermaid` becomes a custom node view:
  rendered SVG inline via vendored mermaid.js (sandboxed config).
- Single-click selects; double-click opens a side panel with diagram source
  and live re-render.
- Parse errors display in the panel; the block falls back to code-block
  styling so source is never trapped.
- HTML export inlines the rendered SVG.

### Images

- Insert opens native file dialog (Go side); file copied to
  `<docname>-assets/` beside the document (created on demand), inserted as a
  relative path. Documents stay portable; absolute paths are never written.
- Paste and drag-drop of image data follow the same path.

### Tabs

- Each tab = one document (path, markdown text, dirty flag).
- Editor instances created lazily per tab, suspended when hidden.

## Ribbon & Interaction Model

Word-inspired ribbon (plain HTML/CSS) above the editor surface:

- **File** — New, Open, Save, Save As, Close Tab, Export (HTML / PDF),
  Recent Files.
- **Home** — Paragraph style dropdown (Normal, H1–H6, Quote, Code Block);
  inline group (Bold, Italic, Strikethrough, Inline Code, Link); list group
  (Bullet, Numbered, Task list, Indent/Outdent).
- **Insert** — Table (grid-picker, Word-style), Image, Code Block (language
  picker), Mermaid Diagram (starter template), Link, Horizontal Rule.
- **View** — WYSIWYG / Raw toggle (also Cmd/Ctrl+E), light/dark theme,
  focus mode (dims ribbon).
- **Contextual: Table Tools** — appears only when cursor is in a table:
  add/remove row/column, per-column alignment, delete table.

Interaction principles:

- Typora-style live rendering: markdown shortcuts (`# `, `- `, `**bold**`)
  transform as you type.
- Every ribbon action has a keyboard shortcut, shown in tooltips.
- Ribbon reflects cursor context (Bold active inside bold, style dropdown
  shows current heading), driven by ProseMirror selection state.
- Status bar: word/character count, mode, file path, save state.

## Export

- **HTML:** serialize document to HTML in the webview (mermaid inlined as
  SVG), pass string to Go, write file via native save dialog.
- **PDF:** render print-ready HTML and invoke the webview's `window.print()`;
  user selects the OS PDF printer (macOS Save as PDF, Microsoft Print to PDF,
  CUPS). No PDF library required.
- **DOCX:** deferred — requires a Go docx generator; later milestone.

## Error Handling

- File I/O errors surface as native message dialogs; document stays open and
  dirty. Nothing is lost silently.
- Closing a dirty tab or quitting prompts Save / Don't Save / Cancel.
- Atomic saves: write temp file in same directory, then rename.
- Mermaid parse errors: shown in the diagram panel, block degrades to code
  view, editor never crashes.
- Frontend exception guard: fall back to raw mode with current markdown
  intact; error logged to a Go-side log file.

## Testing

No Node anywhere in the test strategy.

- **Go unit tests** (`go test`): document model, dirty tracking, atomic save,
  image asset copying, exporter output. Table-driven, no external deps.
- **Round-trip corpus:** markdown fixtures covering every GFM construct,
  mermaid blocks, and adversarial nesting. Run parse→serialize→parse via
  `chromedp` (pure Go) driving system Chrome against the frontend; assert
  stabilization and byte-equality for canonical fixtures.
- **Smoke suite (chromedp):** app boots, ribbon inserts a table, mode toggle
  preserves content.

## Milestones

1. **Walking skeleton** — Wails window, Milkdown loads a hardcoded doc,
   open/save via native dialogs, raw toggle works.
2. **Round-trip solid** — GFM + tables + task lists + code highlighting,
   fixture corpus green.
3. **Ribbon complete** — all tabs, contextual table tools, shortcuts.
4. **Mermaid** — node view, source panel, error states.
5. **Images + Export** — asset copying, HTML export, PDF via print pipeline.
6. **Polish** — themes, focus mode, status bar, packaging for three OSes.
