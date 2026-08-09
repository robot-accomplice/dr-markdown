// The exact spellings the vendored editor strips from the parsed document,
// plus the double-space variant used to carry them through it. Taken from the
// bundle, which matches `html` nodes whose trimmed value is one of the first
// four — case-sensitively, which is why `<BR>` survives untouched.
const STRIPPED_BREAKS = ['<br>', '<br/>', '<br />', '<br >']
const BREAK_SENTINEL = '<br  >'
const BREAK_LIKE = /<br\s*\/?\s*>/g

// Crepe writes `<br />` to represent an empty paragraph, and strips those same
// spellings when parsing so its own marker round-trips. Genuine `<br>` written
// by the user is collateral: it is removed from the document and the words on
// either side are joined, so `run make<br>then sign` becomes `run makethen
// sign`. That is content destruction rather than restyling, and `<br>` is the
// standard GFM idiom for a line break inside a table cell.
//
// Every break-like tag is swapped for the double-space spelling, which is not
// in the stripped set and therefore survives, and the originals are restored in
// document order on the way out — so the user gets back the exact form they
// wrote. The sentinel is deliberately the same length as the longest spelling
// it stands in for: a longer placeholder inflates the column padding the
// serializer computes for a table, turning a content fix into a layout rewrite.
//
// A break the user's own document already spelled `<br  >` is queued too, so it
// cannot be mistaken for a sentinel and consume another break's slot.
//
// TRADE-OFF, deliberate: while editing, the tag shows as a literal rather than
// as a line break. That is already how this editor displays every other inline
// tag — `<span>` and `<kbd>` render as literal text too — so it is consistent
// with the surrounding behaviour rather than a new wart, and it replaces silent
// deletion with something visible and reversible. The real fix is schema-level
// and is scoped in docs/decisions/2026-08-08-markdown-fidelity-scope.md.
//
// Known limit, shared with alt-text restoration: adding or removing a break
// inside the editor shifts the remaining originals by one, because the queue is
// positional. Every spelling is still a valid line break, so the failure is a
// changed spelling rather than lost content.
export const breaks = {
  name: 'breaks',
  capture(markdown) {
    const state = []
    const substituted = markdown.replace(BREAK_LIKE, (match) => {
      if (!STRIPPED_BREAKS.includes(match) && match !== BREAK_SENTINEL) return match
      state.push(match)
      return BREAK_SENTINEL
    })
    return { state, markdown: substituted }
  },
  restore(text, state) {
    const queue = [...state]
    return text.replaceAll(BREAK_SENTINEL, () => queue.shift() ?? '<br>')
  },
}
