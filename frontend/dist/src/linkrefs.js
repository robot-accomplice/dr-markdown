// Link reference definitions survive a round trip through the vendored editor.
//
// The editor resolves `[text][label]` against its definition and serializes an
// inline `[text](url)`, then drops every definition — including ones nothing
// referenced. A document that keeps its URLs in a bibliography at the bottom
// loses that bibliography on the first edit, and every reference in the body is
// rewritten. Unused definitions vanish with no visible change at all, which is
// the worst shape of the bug: nothing on screen tells the user anything went.
//
// The obvious fix — strip definitions before the editor sees them, the way
// frontmatter is handled — does not work, and that was measured rather than
// assumed. Without its definition, `[spec][s]` is not a link, so the serializer
// escapes it to `\[spec]\[s]`. Removing the definitions trades a rewrite for a
// different rewrite.
//
// So the definitions stay, the editor inlines them (which keeps links working
// while editing, and is the better editing experience), and both the reference
// syntax and the definition block are restored on the way out.

// A definition line: `[label]: destination "optional title"`, up to three
// leading spaces per CommonMark.
const DEFINITION = /^ {0,3}\[([^\]]+)\]:[ \t]*(\S+)(?:[ \t]+("[^"]*"|'[^']*'|\([^)]*\)))?[ \t]*$/

// Inline link or image in serialized output: `[text](dest)` / `![alt](dest)`.
const INLINE_LINK = /(!?)\[((?:\\.|[^[\]\\])*)\]\(([^()\s]*)((?:\s+(?:"[^"]*"|'[^']*'))?)\)/g

// A reference use in the source: full `[text][label]`, collapsed `[text][]`, or
// shortcut `[text]` — the last only when a definition exists for that text.
const REFERENCE_USE = /(!?)\[((?:\\.|[^[\]\\])*)\](?:\[([^\]]*)\])?/g

// collectLinkReferences records the definitions and how each one is referenced,
// so serialized inline links can be turned back into references.
export function collectLinkReferences(markdown) {
  const definitions = []
  const byLabel = new Map()
  for (const line of markdown.split('\n')) {
    const match = line.match(DEFINITION)
    if (!match) continue
    const [, label, destination, title] = match
    const record = { label, destination, title: title ?? '', line }
    definitions.push(record)
    byLabel.set(label.toLowerCase(), record)
  }
  if (definitions.length === 0) return null

  // How each definition is used, in document order, so the exact reference
  // style the author wrote is restored rather than normalized to one form.
  const uses = []
  const body = markdown
    .split('\n')
    .filter((line) => !DEFINITION.test(line))
    .join('\n')
  for (const [, bang, text, label] of body.matchAll(REFERENCE_USE)) {
    // `[text][label]` → label; `[text][]` → text; `[text]` → text.
    const resolved = label ? label : text
    const definition = byLabel.get(resolved.toLowerCase())
    if (!definition) continue
    const style = label === undefined ? 'shortcut' : label === '' ? 'collapsed' : 'full'
    // Keep the label as the USE spelled it. Labels match case-insensitively, so
    // `[Spec][S]` against `[s]: ...` is valid and rewriting it to the
    // definition's casing would be a gratuitous change to the user's text.
    uses.push({ destination: definition.destination, label, style, image: bang === '!' })
  }
  return { definitions, uses }
}

// restoreLinkReferences turns inline links back into references and re-appends
// the definition block.
//
// Matching is by destination and consumes a queue in document order, so a link
// the user adds later that happens to share a URL is not silently converted to
// reference syntax — only as many occurrences as were references to begin with.
export function restoreLinkReferences(markdown, recorded) {
  if (!recorded) return markdown
  const pending = new Map()
  for (const use of recorded.uses) {
    if (!pending.has(use.destination)) pending.set(use.destination, [])
    pending.get(use.destination).push(use)
  }

  const body = markdown.replace(INLINE_LINK, (whole, bang, text, destination) => {
    const queue = pending.get(destination)
    if (!queue?.length) return whole
    const use = queue.shift()
    if (use.style === 'shortcut') return `${bang}[${text}]`
    if (use.style === 'collapsed') return `${bang}[${text}][]`
    return `${bang}[${text}][${use.label}]`
  })

  // Definitions are re-appended at the end, which is byte-exact for the
  // overwhelmingly common convention of keeping them there. A document that
  // interleaved them has them collected at the bottom instead — a move, not a
  // loss, and still better than deletion. Unused definitions are included:
  // those are the ones whose disappearance is invisible.
  const block = recorded.definitions.map((d) => d.line).join('\n')
  // A document that is nothing BUT definitions has no body to separate from,
  // and prepending a separator there produced leading blank lines that were not
  // in the file.
  if (body.trim() === '') return block + '\n'
  const separator = body.endsWith('\n') ? '\n' : '\n\n'
  return body + separator + block + '\n'
}
