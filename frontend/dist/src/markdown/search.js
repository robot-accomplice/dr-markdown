// Finding and replacing text in a markdown document.
//
// Every match is an offset range into the MARKDOWN SOURCE, and nothing here
// knows about a textarea, the editor, or the DOM. That is the whole design.
//
// The document is shown in three substrates — a textarea in Raw, a ProseMirror
// document in Formatted, and both at once in Split — and a search that ran over
// whichever surface happened to be visible would give three different answers
// for one document, with Split the worst case: the same file, searched twice,
// disagreeing with itself about how many matches exist. The source is the one
// coordinate every mode shares, so it is the only one a match is expressed in.
// Each surface is responsible for revealing a source range in its own terms.
//
// Replacement is likewise a string-to-string transform on the source, the same
// shape as tables.js, fences.js and commands.js. That matters for more than
// consistency: an edit made through the editor re-serializes the whole
// document, so a replace-all driven through the editor could rewrite lines the
// user never touched.
//
// This used to claim that replace therefore "carries exactly the risk every
// other document command already carries, and no more". That was true about
// RE-SERIALIZATION and false about BLAST RADIUS, and the difference is the whole
// hazard. Every other remounting command edits one construct under the cursor;
// Replace All is the only single click in this product that rewrites the entire
// document. Applying it also remounts the editor, which discards ProseMirror's
// undo history — so the reversal the Edit menu promises could not fire exactly
// where the damage was largest. The caller is responsible for keeping a way
// back; see the replace snapshot in app.js.

// escapeForRegExp quotes every character RegExp gives meaning to, so a literal
// search for `a.b` does not match `axb` and a search for `(` is not a syntax
// error.
function escapeForRegExp(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// compileQuery builds the RegExp a search runs on, or throws.
//
// It throws rather than returning null for an unusable pattern because the
// alternative is a search box that silently reports "no matches" for a typo in
// a regular expression, which is indistinguishable from a document that really
// does not contain the text. The caller shows the message.
export function compileQuery(query, { caseSensitive = false, wholeWord = false, regex = false } = {}) {
  if (!query) return null
  const body = regex ? query : escapeForRegExp(query)
  // \b is a word BOUNDARY, so it only means anything next to a word character.
  // Wrapping a pattern that starts or ends with punctuation in \b would match
  // nothing at all, which reads as a broken search rather than a refused one.
  const bounded = wholeWord ? `\\b(?:${body})\\b` : body
  try {
    return new RegExp(bounded, caseSensitive ? 'gu' : 'giu')
  } catch (error) {
    throw new Error(`invalid search pattern: ${error.message}`)
  }
}

// findMatches returns every match as { start, end } offsets into text.
export function findMatches(text, query, options = {}) {
  const pattern = compileQuery(query, options)
  if (!pattern) return []
  const source = String(text ?? '')
  const matches = []
  let match
  while ((match = pattern.exec(source)) !== null) {
    matches.push({ start: match.index, end: match.index + match[0].length })
    // A pattern can match the empty string — `a*` and `^` both do. Without this
    // the exec loop never advances and the browser hangs on a search box
    // keystroke, which is a worse failure than any wrong result.
    if (match[0].length === 0) pattern.lastIndex += 1
    // Bound the work a single keystroke can cause. A short pattern against a
    // large document is an ordinary thing to type on the way to a longer one.
    if (matches.length >= MATCH_LIMIT) break
  }
  return matches
}

// Matches beyond this are not counted. The count is a navigation aid, and no
// one navigates ten thousand matches one at a time; the cost of finding them is
// paid on every keystroke.
export const MATCH_LIMIT = 10000

// nextMatchIndex returns which match to move to from a caret at `offset`.
//
// `forward` picks the first match beginning at or after the caret, so typing a
// query jumps to the match under the cursor rather than back to the top of the
// document. Both directions wrap.
export function nextMatchIndex(matches, offset, forward = true) {
  if (!matches.length) return -1
  if (forward) {
    const found = matches.findIndex((match) => match.start >= offset)
    return found === -1 ? 0 : found
  }
  for (let i = matches.length - 1; i >= 0; i--) {
    if (matches[i].start < offset) return i
  }
  return matches.length - 1
}

// replaceMatch replaces one match and returns the new document.
//
// The replacement is LITERAL, in regex mode too: `$1` inserts a dollar sign and
// a one. Capture-group substitution is a second syntax to learn in a box that
// mostly holds ordinary words, and it silently mangles any replacement text
// that happens to contain a dollar sign.
export function replaceMatch(text, match, replacement) {
  const source = String(text ?? '')
  if (!match || match.start < 0 || match.end > source.length || match.start > match.end) return source
  return source.slice(0, match.start) + String(replacement ?? '') + source.slice(match.end)
}

// replaceAllMatches returns { text, count }.
//
// Replacements are applied from the END backwards so that each one's offsets
// are still valid when it is applied: replacing left to right shifts every
// later match by the difference in length, and a naive forward loop corrupts
// the document whenever the replacement is not the same length as the match.
// `capped` reports that the scan stopped at MATCH_LIMIT, so occurrences beyond
// it were NOT replaced. Returning the count alone made that indistinguishable
// from a complete pass: "Replaced 10000 matches" on a document with more, with
// the rest silently left behind. A partial rewrite the user believes is total is
// worse than a refused one.
export function replaceAllMatches(text, query, replacement, options = {}) {
  const source = String(text ?? '')
  const matches = findMatches(source, query, options)
  let out = source
  for (let i = matches.length - 1; i >= 0; i--) {
    out = replaceMatch(out, matches[i], replacement)
  }
  return { text: out, count: matches.length, capped: matches.length >= MATCH_LIMIT }
}

// sourceBlockIndex returns which top-level block an offset falls in, counting
// blocks as runs of lines separated by blank lines.
//
// This is how a source offset is revealed in the FORMATTED surface, which has
// no source offsets: a position in the editor's model is a different coordinate
// system, and mapping between them precisely is a separate piece of work
// (probe/source-range-patching). Block granularity is enough to put the match on
// screen, is derived from the same source string as the match itself, and — the
// part that matters — cannot disagree with the match COUNT, which is what a
// second search over the rendered DOM would have done.
export function sourceBlockIndex(text, offset) {
  const source = String(text ?? '')
  const target = Math.max(0, Math.min(offset, source.length))
  let index = -1
  let inBlank = true
  let cursor = 0
  for (const line of source.split('\n')) {
    if (line.trim() === '') {
      inBlank = true
    } else {
      if (inBlank) index += 1
      inBlank = false
    }
    // The offset's OWN line has to be counted before the answer is returned.
    // Counting only the lines before it puts every match at the start of a
    // block in the previous block, which is exactly where a match found at the
    // start of a paragraph lands.
    if (target <= cursor + line.length) return Math.max(0, index)
    cursor += line.length + 1
  }
  return Math.max(0, index)
}
