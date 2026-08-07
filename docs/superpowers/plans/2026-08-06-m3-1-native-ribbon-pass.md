# Dr. Markdown — M3.1 Native Ribbon Pass

**Date:** 2026-08-06
**Status:** In progress

## Architecture

M3.1 remains a macOS Wails application remediation pass. The embedded webview is
an implementation detail; the user-facing behavior should be that of a native
document editor.

The pass has two priorities:

1. **Native-app behavior boundary**
   - Suppress browser-default context menus and drag/drop navigation.
   - Keep chrome controls non-selectable while preserving text selection inside
     the editor surfaces.
   - Avoid UI that exposes missing concepts such as identity, tours, fake
     recents, or sharing.

2. **Ribbon overhaul**
   - Replace arbitrary all-at-once command groups with a tabbed, document-editor
     ribbon.
   - Use Word as an interaction maturity reference, not as a clone target.
   - Keep only backed controls visible.
   - Remove contextual table mutation controls from the ribbon; table editing
     moves to in-page controls in a later M3.1 step.
   - Distinguish inline code from fenced code block in label and placement.
   - Use Mermaid Diagram consistently.

## Verification

- Existing round-trip, file-flow, dirty-state, tab, and shortcut tests must pass.
- New e2e coverage should prove browser-default context menus are suppressed.
- New e2e coverage should prove the ribbon tab switcher exposes the expected
  backed command panels.
