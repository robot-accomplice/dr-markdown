# Dr. Markdown — Concept Gap Map

**Date:** 2026-08-06
**Status:** Active gap map

## Context

`~/Downloads/Dr. Markdown Editor Concepts.zip` is the product
direction for M3.1 and beyond. It is explicit about the desired macOS editor
surface, but it also implies functionality the current app does not yet have.

This document prevents the concept port from being misrepresented as complete.

## Current Pass Scope

The current implementation pass may port the static shell and wire existing
behavior into it:

- custom app chrome inside the Wails webview
- ribbon visual system and backed command buttons
- file rail display for open in-memory documents
- outline/status panels with honest placeholder values
- blank-document empty state
- formatted/raw mode switch using the existing editors

## Missing Subsystems Implied by the Concept

- **Split mode:** two synchronized panes with source and formatted preview.
- **Outline extraction:** parse headings from the active markdown and navigate to
  them.
- **Raw syntax tinting:** language-aware markdown highlighting with line-number
  and marker display controls.
- **Code block highlighting:** bundled highlighter for common languages in
  formatted code blocks.
- **Settings modal:** persistent editor preferences, including default mode and
  monospace font.
- **Insert popover:** command menu with shortcuts and block insertion behavior.
- **File rail persistence:** real recent/workspace document list, not fabricated
  examples.
- **In-page table controls:** table creation from the ribbon, mutation near the
  table.
- **Image handling:** native file selection, asset copying, paste/drop handling,
  and placeholders.
- **Mermaid diagrams:** rendered node view, editing panel, and diagram-type
  helper.
- **Native shell polish:** frameless/custom titlebar decisions, menus, context
  actions, and app-level shortcuts.

## Rule

Do not call the concept implemented until the relevant subsystem exists and has
verification. A visually matching placeholder must be labeled as a placeholder
in planning and must not fake user data or unavailable product behavior.
