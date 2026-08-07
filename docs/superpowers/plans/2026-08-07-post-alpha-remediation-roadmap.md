# Post-Alpha Remediation Roadmap

## Objective

Close the remaining alpha gaps without weakening the architecture. The target
remains a native macOS application moving toward Microsoft Word-level document
editing features for a WYSIWYG/raw-toggleable open source markdown editor.

## Planning Principles

- Fix architectural blockers before layering features on top of them.
- Every visible enabled control must remain backed by real behavior.
- Markdown on disk remains the source of truth unless a future document model
  decision explicitly changes that.
- Each milestone starts with architecture and tests, then implementation.
- The Go unit coverage target remains 80% LOC coverage, but it must be reached
  through extracted seams rather than tests that invoke Wails runtime globals.

## GitHub Tracking

- Roadmap tracker: <https://github.com/robot-accomplice/dr-markdown/issues/5>
- M5 Native State, Preferences, And Recents Foundation: <https://github.com/robot-accomplice/dr-markdown/issues/8>
- M6 Editable Source Token Layer: <https://github.com/robot-accomplice/dr-markdown/issues/7>
- M7 Cursor-Precise Document Block Model: <https://github.com/robot-accomplice/dr-markdown/issues/6>
- M8 Image Asset Management: <https://github.com/robot-accomplice/dr-markdown/issues/9>
- M9 Comments And Review Workflow: <https://github.com/robot-accomplice/dr-markdown/issues/12>
- M10 Direct Export Pipeline: <https://github.com/robot-accomplice/dr-markdown/issues/10>
- M11 Share, Sync/Git, And Extensions Architecture: <https://github.com/robot-accomplice/dr-markdown/issues/13>
- M12 Packaging, Signing, Updates, And Platform Hardening: <https://github.com/robot-accomplice/dr-markdown/issues/11>
- Responsive Shell Adaptation: <https://github.com/robot-accomplice/dr-markdown/issues/14>

## Milestone M5 - Native State, Preferences, And Recents Foundation

### Scope

- Extract native dialog, file, window title, and close-guard ports from
  `app.go`.
- Move testable application state decisions out of Wails-bound runtime methods.
- Add a native preference store for session settings.
- Persist and render recent files/workspace entries on the start screen.
- Correct stale README alpha limitation text after the architecture is updated.

### Acceptance

- Unit tests cover close-guard choices, save/save-as routing, title state,
  preference load/save, and recents updates without requiring Wails runtime.
- Non-e2e Go coverage reaches at least 80%.
- E2E verifies settings survive app restart/reload through a bridge stub.
- E2E verifies recent files are recorded after open/save and rendered on the
  start screen.
- `architext validate .` and `go test ./... -count=1` pass.

### Dependencies

- None. This milestone should happen before more native features.

## Milestone M6 - Editable Source Token Layer

### Scope

- Replace or extend the raw/split source editor with an editable markdown token
  layer.
- Keep Highlight.js only where it remains useful, but stop relying on a purely
  visual overlay for source semantics.
- Implement raw marker hiding as a backed user preference.
- Preserve scroll synchronization and source editing behavior.

### Acceptance

- E2E verifies raw and split modes can hide markdown markers without changing
  source text.
- E2E verifies source highlighting, selection, editing, and scroll sync still
  work on large documents.
- Unit tests cover language aliasing/token configuration where practical.

### Dependencies

- M5 preference persistence if marker hiding is persisted.

## Cross-Cutting UI Hardening - Responsive Shell Adaptation

GitHub issue: <https://github.com/robot-accomplice/dr-markdown/issues/14>

### Scope

- Define graceful reduced-window behavior for the ribbon, side panels,
  contextual controls, mode controls, and status bar.
- Collapse eligible text buttons to icon-only controls with accessible labels
  and hover tooltips when space is constrained.
- Preserve fixed control dimensions and consistent icon/text spacing at full
  size.
- Prevent hidden or partially clipped primary controls.

### Acceptance

- E2E/visual tests cover default, medium, and narrow desktop widths.
- Ribbon controls remain visible, intentionally collapsed, or grouped in
  overflow controls without partial clipping.
- Icon-only controls expose tooltips and accessible labels.
- README screenshots are refreshed when visible UI changes.

### Dependencies

- M5 screenshot refresh workflow.

## Milestone M7 - Cursor-Precise Document Block Model

### Scope

- Introduce a block identity/selection model for rendered markdown blocks.
- Attach contextual controls to the acted-on table, code block, Mermaid Diagram,
  image, heading, list, or paragraph.
- Replace first-detected-block behavior with selected-block behavior.
- Add stable handles, hover tools, keyboard focus, and accessibility state for
  block-local operations.

### Acceptance

- E2E verifies multiple tables/code blocks/diagrams can be edited
  independently from the rendered surface.
- E2E verifies keyboard and pointer block selection produce the same target.
- E2E verifies contextual controls never mutate an unselected sibling block.
- Visual baseline screenshots cover selected and hovered block states.

### Dependencies

- M5 for testability expectations.
- M6 if raw/source selection state must map directly to rendered blocks.

## Milestone M8 - Image Asset Management

### Scope

- Add native image selection and drag/drop import.
- Define asset storage policy for local documents: relative path, copied asset
  folder, overwrite behavior, and missing-file handling.
- Render image blocks with contextual sizing, alt text, replace, reveal, and
  delete controls.
- Preserve markdown portability.

### Acceptance

- Unit tests cover asset path normalization, copy policy, collision handling,
  and markdown rewrite behavior.
- E2E verifies insert, drag/drop, resize/alt edit, replace, delete, reopen, and
  missing-asset states.
- Export/print includes images where the backing file exists.

### Dependencies

- M5 native file/dialog ports.
- M7 block-local controls.

## Milestone M9 - Comments And Review Workflow

### Scope

- Define comment anchors against markdown ranges or stable block IDs.
- Add comment create, edit, resolve, delete, and navigation flows.
- Build review side-panel behavior and visible anchors in the document.
- Persist comments either in sidecar metadata or an approved embedded format.

### Acceptance

- Architecture decision records the comment persistence model.
- Unit tests cover anchor updates across markdown edits.
- E2E verifies comment lifecycle, navigation, resolved state, and reopen.
- Comments never corrupt saved markdown.

### Dependencies

- M5 persistence foundation.
- M7 block/range selection model.

## Milestone M10 - Direct Export Pipeline

### Scope

- Add direct PDF file generation or a native print-to-PDF automation path if
  Wails/macOS exposes a reliable API.
- Add HTML export from the same formatted document rendering pipeline.
- Include code highlighting, Mermaid diagrams, images, page breaks, and print
  styles.
- Keep the current OS print dialog path as Print.

### Acceptance

- Integration or e2e tests verify generated PDF and HTML artifacts contain
  headings, paragraphs, highlighted code, Mermaid diagrams, tables, and images.
- Export commands do not mutate markdown or dirty state.
- Export errors surface through native dialogs without data loss.

### Dependencies

- M8 for image completeness.
- M5 native dialog/file ports.

## Milestone M11 - Share, Sync/Git, And Extensions Architecture

### Scope

- Split this into explicit architecture decisions before implementation:
  Share, Sync/Git, and Extensions are separate systems.
- Define local-first Git behavior: repository detection, status display, commit,
  diff, conflict states, and authentication boundaries.
- Define macOS share behavior and fallback when native APIs are unavailable.
- Define extension execution, permissions, isolation, and distribution model.

### Acceptance

- Architecture decisions exist before UI controls are enabled.
- Each subsystem has a threat model and trust boundary.
- Any enabled UI has e2e coverage and no decorative placeholders.

### Dependencies

- M5 native ports and persistence.
- M9 if sharing/review comments are included.

## Milestone M12 - Packaging, Signing, Updates, And Platform Hardening

### Scope

- Add Developer ID signing and notarization for macOS.
- Define update channel policy for alpha/beta/stable.
- Build reproducible release artifacts.
- Evaluate Windows/Linux packaging only after macOS alpha stabilizes.
- Add release smoke tests for packaged app launch and basic file flow.

### Acceptance

- CI or release script builds signed/notarized macOS artifacts.
- Release artifacts include checksums and release notes.
- Packaged app smoke test passes before release publication.
- Gatekeeper install/open path is documented and verified.

### Dependencies

- M5 for testable app shell behavior.

## Cross-Milestone Quality Gates

- `architext validate .`
- `node --check frontend/dist/src/app.js`
- `go test ./... -count=1`
- Unit coverage report for non-e2e Go packages.
- Visual baseline e2e for any changed screen.
- Release Truth update for completed, deferred, blocked, or reprioritized work.

## Recommended Order

1. M5 Native State, Preferences, And Recents Foundation
2. M6 Editable Source Token Layer
3. M7 Cursor-Precise Document Block Model
4. M8 Image Asset Management
5. M9 Comments And Review Workflow
6. M10 Direct Export Pipeline
7. M11 Share, Sync/Git, And Extensions Architecture
8. M12 Packaging, Signing, Updates, And Platform Hardening

This order is deliberate. M5 removes the most expensive architectural blocker,
M6 fixes the source editor limitation, M7 gives later document features a
correct targeting model, and M8-M11 then add document/product capabilities on
top of stable foundations.
