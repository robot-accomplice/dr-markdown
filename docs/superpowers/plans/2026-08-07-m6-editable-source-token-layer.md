# M6 Editable Source Token Layer

GitHub issue: <https://github.com/robot-accomplice/dr-markdown/issues/7>

## Objective

Back Raw and Split source marker hiding without changing markdown source text.
The implemented slice extends the existing synchronized source layer with
explicit markdown marker tokens and hides those marker glyphs by visibility so
textarea caret positions remain aligned with the raw source.

## Architecture

1. Tokenize markdown source markers in the highlighted source layer.
   - Verify marker spans exist for headings, links, lists, quotes, inline code,
     emphasis, and fenced code delimiters.
2. Add a persisted raw option for marker hiding.
   - Verify Raw mode exposes a backed toggle and Settings persists the option
     through the preference envelope.
3. Apply the same source rendering option in Split mode.
   - Verify Split source hides marker glyphs without changing the textarea
     value.

## Tradeoff

This does not reflow the source text when markers are hidden. Marker glyphs keep
their layout width, which is deliberate: the visible token layer stays aligned
with the editable textarea and caret positions. A later richer editor engine can
revisit reflowing marker decorations if it can preserve selection fidelity.

## Test Plan

- Red first: e2e tests for Raw marker toggle and Split marker hiding.
- Green: source highlighter tokenization, raw/split option wiring, CSS marker
  visibility.
- Refresh README screenshots with `go run ./tools/screenshots`.

## Acceptance

- Raw mode includes a backed Hide markers toggle.
- Raw/Split marker hiding does not mutate markdown source.
- Source highlighting for headings and links remains present.
- README screenshots are refreshed from the current UI.
