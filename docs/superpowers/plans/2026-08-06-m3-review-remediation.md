# Dr. Markdown — M3 Review Remediation

**Date:** 2026-08-06
**Status:** Proposed

## Context

The target remains a native macOS application built with Wails, not a website.
The product objective is a Microsoft Word level of authoring features while
keeping markdown as the canonical document format and preserving a raw markdown
escape hatch.

The first M3 pass added a ribbon-shaped command surface, tabs, shortcuts, and
basic table tools. Review showed that this is not yet acceptable as the M3
finish line because it still behaves like a generic web UI and some command
grouping/verbiage is not aligned with a native document editor.

## Architectural Drivers

- Native macOS behavior must take precedence over browser defaults.
- App launch and New must start with a blank untitled document, not an example
  or template document.
- Ribbon commands must be named by author intent, not implementation detail.
- Code blocks need a language model, native in-app syntax highlighting for the
  majority of common programming and markup languages, and a configurable
  monospace font path.
- Tables should be inserted from the ribbon but managed in-place at the table.
- Mermaid should be treated as a diagram feature, with guided creation deferred
  until the diagram milestone.

## Required M3 Remediation

1. **Command taxonomy**
   - Rename and regroup ribbon items around mature document-authoring tasks.
     Microsoft Word is the interaction-quality reference, not a product to
     clone.
   - Use familiar categories only where they fit markdown authoring. Do not
     expose meaningless Word-only groups, and do not copy Word's information
     architecture when markdown needs a clearer model.
   - Distinguish inline code from fenced code block with explicit labels.
   - Rename Mermaid to Mermaid Diagram.
   - Add mature command clusters over time: clipboard, typography, paragraph,
     lists, alignment, styles, reviewing/commenting, editing mode, and share or
     export when those features exist.

2. **Document startup**
   - Remove the example welcome document from application boot.
   - Launch into a familiar native start center with templates and recents,
     similar to Word's macOS home screen.
   - Make Blank Document the primary template action.
   - Do not enter the editor canvas until the user creates or opens a document.
   - Keep templates as a future explicit user action, not the default state.
   - Do not fabricate recent files; show an honest empty state until recent-file
     persistence exists.
   - Do not show user identity controls or tour entries until those concepts are
     implemented.

3. **Code authoring**
   - Add a language selector for code block insertion.
   - Preserve selected language in fenced code info strings.
   - Add bundled, offline syntax highlighting for the majority of common code
     languages in WYSIWYG code blocks.
   - Add matching language-aware highlighting in raw mode where practical; raw
     mode should at minimum preserve the selected language and avoid misleading
     plain-text treatment.
   - Add selectable display fonts, with separate handling for prose and code.
   - Treat highlighting assets as vendored application assets: no CDN, no
     runtime downloads, no Node.js dependency in the normal build/test path.

4. **Native app behavior**
   - Capture context-menu behavior so right-click does not expose browser-page
     defaults.
   - Replace browser-native interactions that leak the webview implementation
     with app-owned behavior.

5. **Table interaction model**
   - Keep table creation in Insert.
   - Remove table mutation commands from the ribbon.
   - Add on-the-spot table controls near the table for row/column/alignment
     operations.

6. **Mermaid naming and future path**
   - Use Mermaid Diagram consistently.
   - Defer a guided diagram-type helper to the Mermaid milestone unless it is
     needed to complete the current interaction model.

## Recommendation

Do not start M4 yet. Treat the next work item as **M3.1 — Native Ribbon and
Authoring Controls**. M3.1 should replace the provisional markdown-transform
command surface with app-grade controls where user intent and native behavior
are clear.
