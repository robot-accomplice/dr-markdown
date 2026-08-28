// Fenced code blocks: reading a fence's info string and rewriting a fence's
// language or its body. Mermaid is a fenced block with a known language rather
// than a separate construct, so it is handled here rather than in its own
// module — `onlyMermaid` and `excludeMermaid` exist so callers can ask for the
// diagram or for the code, from the same scan.
//
// normalizeLanguage comes from the highlighter because a fence's info string
// and a highlighter's language id must agree — `js` and `javascript` name the
// same language, and a fence rewritten to one while the highlighter expects
// the other renders unhighlighted.
import { normalizeLanguage } from '../highlighter.js'

export function firstCodeFenceLanguage(md, { excludeMermaid = false, onlyMermaid = false } = {}) {
  return firstCodeFenceDescriptor(md, { excludeMermaid, onlyMermaid })?.language ?? ''
}

export function firstCodeFenceDescriptor(md, { excludeMermaid = false, onlyMermaid = false } = {}) {
  let index = -1
  let inFence = false
  for (const line of md.split('\n')) {
    const match = line.match(/^```\s*([A-Za-z0-9_+#.-]*)/)
    if (!match) continue
    if (inFence) {
      inFence = false
      continue
    }
    inFence = true
    index++
    const language = normalizeLanguage(match[1] || 'text')
    if (excludeMermaid && language === 'mermaid') continue
    if (onlyMermaid && language !== 'mermaid') continue
    return { index, language }
  }
  return null
}

export function rewriteCodeFenceLanguage(md, fenceIndex, language) {
  if (!Number.isInteger(fenceIndex)) return md
  const lines = md.split('\n')
  const normalized = normalizeLanguage(language || 'text') || 'text'
  let currentFenceIndex = -1
  let inFence = false
  for (let i = 0; i < lines.length; i++) {
    const match = lines[i].match(/^(```)\s*([A-Za-z0-9_+#.-]*)/)
    if (!match) continue
    if (inFence) {
      inFence = false
      continue
    }
    inFence = true
    currentFenceIndex++
    if (currentFenceIndex !== fenceIndex) continue
    if (normalizeLanguage(match[2] || 'text') === 'mermaid') continue
    lines[i] = `${match[1]}${normalized}`
    break
  }
  return lines.join('\n')
}

export function containsMermaidDiagram(md) {
  return firstCodeFenceLanguage(md, { onlyMermaid: true }) === 'mermaid'
}

export function rewriteMermaidFenceSource(md, fenceIndex, source) {
  if (!Number.isInteger(fenceIndex)) return md
  const lines = md.split('\n')
  let currentFenceIndex = -1
  let inFence = false
  for (let i = 0; i < lines.length; i++) {
    const match = lines[i].match(/^```\s*([A-Za-z0-9_+#.-]*)/)
    if (!match) continue
    if (inFence) {
      inFence = false
      continue
    }
    currentFenceIndex++
    const language = normalizeLanguage(match[1] || 'text')
    inFence = true
    if (currentFenceIndex !== fenceIndex || language !== 'mermaid') continue
    let end = i + 1
    while (end < lines.length && !/^```/.test(lines[end])) end++
    if (end >= lines.length) return md
    lines.splice(i + 1, end - i - 1, ...source.split('\n'))
    return lines.join('\n')
  }
  return md
}

// The body of every mermaid fence, in document order.
//
// The editor draws these before it mounts, so its code-block preview hook can
// answer synchronously: that hook runs during the node view's render and the
// node view mounts a COPY of what it is handed, so an element filled in later
// is filled in after it has stopped being the one on screen.
export function mermaidFenceSources(md) {
  const sources = []
  const lines = md.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const match = lines[i].match(/^```\s*([A-Za-z0-9_+#.-]*)/)
    if (!match) continue
    let end = i + 1
    while (end < lines.length && !/^```/.test(lines[end])) end++
    if (normalizeLanguage(match[1] || 'text') === 'mermaid') {
      sources.push(lines.slice(i + 1, end).join('\n'))
    }
    i = end
  }
  return sources
}

export function fencedLanguages(md) {
  return md.split('\n')
    .map((line) => line.match(/^```\s*([A-Za-z0-9_+#.-]+)/)?.[1] || '')
    .filter(Boolean)
}

// codeFenceIndexByLine maps the 0-based source line of each OPENING fence to
// its fence index.
//
// The index is only meaningful because rewriteCodeFenceLanguage and
// rewriteMermaidFenceSource count fences the same way, and they are what
// actually edits the document. A renderer that numbered fences by its own
// parse order — counting fences indented inside a list, which `^```` does not
// match — would hand the assistant an index that names a different block than
// the one the user right-clicked, and the rewrite would land on the wrong code.
// So the numbering lives here, next to the rewriters, and has exactly one
// implementation.
//
// A fence this scan cannot see has no entry, and callers pass null rather than
// a guess: no language picker and no assistant on that block is correct, since
// the rewriters could not have found it either.
export function codeFenceIndexByLine(md) {
  const byLine = new Map()
  let index = -1
  let inFence = false
  md.split('\n').forEach((line, lineNumber) => {
    if (!/^```/.test(line)) return
    if (inFence) {
      inFence = false
      return
    }
    inFence = true
    index++
    byLine.set(lineNumber, index)
  })
  return byLine
}
