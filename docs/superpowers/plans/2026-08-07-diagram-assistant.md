# M3.6 Mermaid Diagram Assistant

## Objective

Add a backed Mermaid Diagram creation assistant and live Mermaid rendering so
users can create diagrams from the formatted workflow and see rendered diagram
output instead of raw code blocks.

## Architectural Boundary

The assistant is a frontend insert workflow. The persisted source remains plain
Mermaid fenced markdown, but formatted and split-preview surfaces render Mermaid
blocks through a local vendored Mermaid runtime. The assistant provides
type-specific starter fields and a live preview before insertion.

## Functional Surface

- Visible Mermaid Diagram controls open an assistant instead of inserting an
  arbitrary default graph.
- The assistant presents common Mermaid diagram types with concise descriptions.
- Choosing a type exposes type-appropriate starter fields and updates a live
  rendered preview.
- Confirming inserts a starter `mermaid` fenced code block for that type using
  the selected field values.
- Cancel closes the assistant without mutating markdown.
- Existing Mermaid fenced blocks render as diagrams in formatted mode and split
  preview.

## Deferred Subsystems

- Contextual diagram editing controls.
- Full multi-step diagram construction beyond first-pass type-specific starter
  fields.

## Acceptance

- End-to-end tests cover opening the assistant from Insert, selecting a diagram
  type, editing starter fields, seeing a live rendered preview, inserting a
  starter block, rendering existing Mermaid blocks, and canceling without
  document mutation.
- Existing direct command paths remain available for tests and internal command
  execution.
