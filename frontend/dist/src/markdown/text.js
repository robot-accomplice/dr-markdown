// Document text conventions: line endings, and the name shown for a path.

// Line endings are a property of the FILE, not of the editing surface. Both
// surfaces destroy them: the WYSIWYG editor re-serializes with LF, and a
// textarea normalizes CRLF to LF on input per the HTML spec. So a
// Windows-authored file came back whole-file changed after a one-word edit —
// the loudest possible diff for anyone keeping notes in version control.
//
// Normalizing on the way in and restoring on the way out fixes both surfaces at
// one seam, and keeps every in-memory comparison (dirty tracking, savedText)
// working in a single representation instead of two.
export const CRLF = '\r\n'

export function detectLineEnding(text) {
  return text.includes(CRLF) ? CRLF : '\n'
}

export function toEditorText(text) {
  return text.replace(/\r\n/g, '\n')
}

export function toFileText(text, lineEnding) {
  return lineEnding === CRLF ? text.replace(/\n/g, CRLF) : text
}

// The name shown for a document, from its path. Split on both separators so a
// Windows-authored path does not show its whole directory chain as the title.
//
// The unused `fallback` parameter and its dead ternary were dropped in the
// move: both branches returned 'Untitled.md', so the condition never mattered.
export function titleForPath(path) {
  if (!path) return 'Untitled.md'
  return path.split(/[\\/]/).pop() || path
}
