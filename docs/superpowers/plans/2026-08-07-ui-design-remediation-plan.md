# M4 UI Design Remediation Plan

## Objective

Bring the current Dr. Markdown macOS editor surface back into alignment with the
normative design package while preserving the rule that visible controls must be
backed by real behavior or explicitly absent.

This plan is a draft planning artifact. It is not Release Truth until approved.

## Source Inputs

- `/Users/jmachen/Downloads/Dr. Markdown Editor Concepts.zip`
- `/tmp/dr-markdown-design-audit.jRFuPH/Dr Markdown Handoff/IMPLEMENTATION.md`
- Captured current app screenshots under `/tmp/dr-markdown-current-screens.oxxkAk`
- Current e2e coverage in `e2e/e2e_test.go`

## Architecture Boundary

Most remediation belongs in the frontend shell and rendering adapters:

- `frontend/dist/src/app.js` owns mode orchestration, ribbon actions, assistants,
  split preview rendering, settings, and side panel state.
- `frontend/dist/src/app.css` owns design-token fidelity and rendered markdown
  presentation.
- `frontend/dist/src/editor.js` owns the Milkdown/Crepe WYSIWYG boundary.
- `frontend/dist/src/highlighter.js` and `frontend/dist/src/mermaid-renderer.js`
  own code and Mermaid rendering adapters.
- Go/Wails should only change where native app behavior is required, such as
  preferences persistence or native shell/window behavior.

## Milestone M4.1 — Rendered Markdown Fidelity

### Goal

Formatted mode and Split preview should render markdown with the typography,
block styling, and semantic presentation specified by the design.

### Scope

- Force rendered document typography to Public Sans unless changed through
  document font settings.
- Remove unintended serif leakage from headings/body content.
- Render markdown links and emphasis correctly in Split preview.
- Add design-matching code block shells: bordered block, chrome language header,
  Copy affordance, and highlighted code body.
- Theme Mermaid output to match Dr. Markdown neutral/accent tokens instead of
  Mermaid defaults.
- Bring blockquote/callout, task list, table, and inline code styling closer to
  the specification.

### Acceptance

- E2E checks prove formatted headings/body use the configured document font.
- Split preview renders links/emphasis as formatted HTML, not literal markdown
  syntax.
- Fenced code blocks in formatted and split previews expose a language label and
  Copy button.
- Mermaid SVG output uses configured theme variables compatible with the app
  palette.
- Existing markdown round-trip tests still pass.

### Deferred

- Full Word-level layout controls.
- Contextual editing chrome for every rendered block type.

## Milestone M4.2 — Split Mode Parity

### Goal

Split mode should behave like the design: source and rendered panes are equal,
plainly labeled, and scroll-linked.

### Scope

- Change the formatted pane header to `Formatted · scroll locked`.
- Implement scroll synchronization between source and formatted panes.
- Keep Split source gutter behavior aligned with the design: no raw-mode gutter
  unless explicitly chosen.
- Preserve panel hide/show state across split transitions.
- Verify large documents do not desynchronize or cause layout jumps.

### Acceptance

- E2E test scrolls the source pane and observes formatted pane scroll movement.
- E2E test scrolls the formatted pane and observes source pane scroll movement,
  unless a future unlock control is introduced.
- Header text and pane proportions match the reference render at 1440x900.
- Mode switching preserves markdown exactly.

### Deferred

- Optional split-scroll unlock toggle, unless explicitly requested.

## Milestone M4.3 — Ribbon Completion And Command Taxonomy

### Goal

The ribbon should reach the design package's command maturity without showing
fake controls.

### Scope

- Reconcile tabs: Home, Insert, Format, View, Help, plus Settings in the tab row
  per later user direction.
- Restore or intentionally defer missing command groups:
  Image, Math, Export, Preview, Share, Help.
- Keep label-led icon+text buttons in the Insert group.
- Standardize button widths, icon/text gaps, active state, hover state, and
  command naming.
- Ensure every enabled command is backed by behavior or hidden.
- Add disabled/future controls only if the design decision permits visible
  disabled placeholders.

### Acceptance

- E2E inventory finds no decorative enabled controls.
- Ribbon fit test passes at the default macOS window size.
- Home and Insert tabs expose the expected backed command set.
- Missing design commands each have an explicit release-truth disposition:
  implemented, deferred, or hidden.

### Deferred

- Collaboration-backed Share.
- Full export format matrix if the native export boundary is not yet designed.

## Milestone M4.4 — Settings Completion

### Goal

Settings should match the design structure while maintaining the backed-controls
rule.

### Scope

- Restore the settings navigation structure:
  Editor, Appearance, Markdown flavour, Shortcuts, Sync & Git, Extensions.
- Implement backed Editor settings from the design:
  Default mode, Show syntax markers in Formatted, Editor width, Monospace face,
  Format on save.
- Preserve current backed settings:
  document font, code font, ligatures, soft wrap, line numbers.
- Decide whether future-only categories render disabled explanatory states or
  remain hidden until backed.
- Add a native preference store if settings are expected to survive app relaunch.

### Acceptance

- E2E tests cover Save/Cancel for every visible setting.
- Settings changes apply immediately after Save to formatted, raw, and split
  surfaces as appropriate.
- If persistence is in scope, a boot/reload test proves preferences survive.
- Future categories do not imply functionality that does not exist.

### Deferred

- Sync providers.
- Extension marketplace/runtime.

## Milestone M4.5 — Side Panels And Empty State Polish

### Goal

Panel visibility should feel native and intentional, and the empty state should
match the reference composition.

### Scope

- Replace bare arrow glyphs with integrated disclosure/side-panel controls.
- Decide whether empty state hides the right document panel by default.
- Restore file rail hierarchy from the design: Workspace and `docs/` sections
  where data exists.
- Keep independent left/right hide state across modes and document switches.
- Add accessible names, focus order, and keyboard activation for panel toggles.

### Acceptance

- E2E proves empty-state panel behavior matches the chosen design rule.
- E2E proves hide/show controls are keyboard reachable and do not mutate
  markdown.
- Visual screenshot at 1440x900 matches the empty-state reference within known
  accepted divergences.

### Deferred

- Persistent workspace/recent-file index unless approved for this milestone.

## Milestone M4.6 — Contextual Document Controls

### Goal

Context-sensitive document objects should be managed in the page, not from a
global ribbon-only workflow.

### Scope

- Table insertion remains in the ribbon/Insert menu.
- Table editing moves to on-the-spot controls: add/delete row, add/delete
  column, alignment, selection, and delete table.
- Code blocks expose contextual language selection and Copy.
- Mermaid blocks expose contextual Edit Diagram entry into the assistant.
- Image blocks expose placeholder/drop/select behavior if image support is in
  this milestone.

### Acceptance

- E2E inserts a table and edits it entirely through contextual controls.
- E2E changes an existing code block language without entering Raw/Split.
- E2E opens an existing Mermaid block in the assistant and updates its source.
- Contextual controls do not appear when the cursor is outside the relevant
  block.

### Deferred

- Advanced table styles.
- Diagram wizard beyond starter/edit support.

## Milestone M4.7 — Accessibility And Native Feel Pass

### Goal

The app should stop feeling like a browser surface and meet baseline keyboard,
focus, and semantic expectations.

### Scope

- Audit focus rings and tab order across ribbon, rail, editor, side panels,
  popovers, assistants, and settings.
- Trap focus in dialogs and restore focus on close.
- Ensure popovers close on Escape and outside click.
- Ensure right-click/contextmenu behavior is app-appropriate.
- Audit visible hover/active/disabled states.
- Verify default Wails window size and macOS launch presentation.

### Acceptance

- E2E keyboard tests cover dialogs, popovers, side-panel toggles, and mode
  controls.
- Contextmenu suppression tests pass on the app shell while editor-native
  interactions remain usable.
- Manual Wails launch review confirms no duplicated native chrome and no clipped
  ribbon controls at default size.

### Deferred

- Full screen-reader certification.

## Milestone M4.8 — Print And PDF Export

### Goal

Users should be able to print the current document and export it to PDF through
native macOS-feeling workflows, with output that reflects the formatted document
surface rather than raw browser chrome.

### Scope

- Add backed Print and Export to PDF commands in the ribbon/menu structure.
- Decide command placement with the M4.3 ribbon taxonomy work:
  likely File/Export/Share-adjacent rather than a contextual View action.
- Generate print/PDF output from the formatted document rendering, not the whole
  application shell.
- Define print stylesheet/page rules:
  page margins, document font, code font, headings, tables, blockquotes, code
  block headers, Mermaid diagrams, images, and page-break behavior.
- Support native print dialog invocation where Wails/macOS exposes it.
- Support PDF export either through native print-to-PDF or a dedicated PDF
  generation path if native capture is insufficient.
- Ensure raw and split modes print/export the formatted document by default, with
  a later option for raw-source printing if explicitly needed.

### Acceptance

- E2E or integration coverage verifies Print and Export to PDF commands are
  present only when backed.
- A generated PDF fixture contains formatted headings, paragraphs, highlighted
  code, tables, and Mermaid diagrams without app chrome.
- Print stylesheet excludes ribbon, file rail, outline panel, status bar,
  popovers, modals, and side-panel controls.
- Manual macOS launch review confirms the command opens the expected native
  print/PDF workflow.
- Existing save/export commands do not mutate markdown or dirty state.

### Deferred

- Advanced page setup templates.
- Raw-source print/export mode.
- DOCX export.

## Milestone M4.9 — Visual Regression Baseline

### Goal

Prevent recurring design drift after remediation.

### Scope

- Add deterministic screenshot capture fixtures for:
  formatted, raw, split, empty state, settings, insert popover, code assistant,
  diagram assistant.
- Compare against approved baselines at 1440x900.
- Keep accepted divergences documented beside the baselines.

### Acceptance

- A local visual check command captures all milestone screens.
- CI or local gate fails on material layout regressions once baselines are
  approved.
- Baselines are updated only with explicit approval.

### Deferred

- Pixel-perfect CI until font/rendering determinism is stable on macOS runners.

## Proposed Order

1. M4.1 Rendered Markdown Fidelity
2. M4.2 Split Mode Parity
3. M4.3 Ribbon Completion And Command Taxonomy
4. M4.4 Settings Completion
5. M4.5 Side Panels And Empty State Polish
6. M4.6 Contextual Document Controls
7. M4.7 Accessibility And Native Feel Pass
8. M4.8 Print And PDF Export
9. M4.9 Visual Regression Baseline

## Release Truth Handling

Do not add these items to current Release Truth until the plan is approved.
When approved, create a new release entry or promote selected items into the
current release as planned scope. Each implemented slice must update Architext
nodes/flows/decisions if architecture, data flow, persistence, or module
responsibility changes.

## Verification Gate For Every Slice

- Write failing e2e or unit coverage first.
- Implement the smallest systemic change that satisfies the design requirement.
- Run focused tests for the slice.
- Run `architext validate .` after architecture/release data changes.
- Run `go test ./... -count=1` before marking the slice complete.
