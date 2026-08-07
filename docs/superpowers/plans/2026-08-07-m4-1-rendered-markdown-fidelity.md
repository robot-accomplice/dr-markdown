# M4.1 Rendered Markdown Fidelity

## Objective

Bring formatted mode and Split preview rendering closer to the design package:
document typography, inline markdown semantics, code block chrome, and Mermaid
theme should feel like Dr. Markdown instead of browser/editor defaults.

## Architectural Boundary

Rendered markdown fidelity belongs to the frontend rendering adapters and CSS
tokens. The canonical document remains markdown text. Rendering changes must not
mutate source markdown, dirty state, local file paths, or Wails document IO.

## Functional Surface

- Formatted document content uses the configured document font instead of
  leaking serif editor defaults.
- Split preview renders common inline markdown semantics, including emphasis,
  strong text, inline code, and links.
- Fenced code blocks render through a shared code-block shell with language label
  and Copy affordance.
- Code block shells work in Split preview and formatted WYSIWYG mode.
- Mermaid rendering uses Dr. Markdown design tokens rather than Mermaid's default
  yellow/purple theme.

## Coverage Goal

Go unit coverage should continue moving toward 80% LOC coverage. This slice is
mostly frontend behavior, so its primary checks are e2e tests. Any Go changes
must include unit coverage.

## Acceptance

- E2E tests fail before implementation for split inline rendering and code block
  chrome.
- E2E tests pass after implementation.
- Markdown round-trip tests continue to pass.
- `architext validate .` passes after Release Truth updates.
- `go test ./... -count=1` passes before marking complete.
