// markdown -> nodes for the surfaces that are not the editor: the split preview
// and the print/PDF surface.
//
// These used to share a 43-line hand-written renderer that matched headings and
// fences with regular expressions. Measured across sixteen ordinary constructs,
// SIXTEEN were wrong — a GFM table came out as three paragraphs of pipes, an
// ordered list lost its numbers, nested lists flattened, task lists showed
// literal brackets, emphasis inside a heading or quote stayed as asterisks, and
// a wrapped paragraph split in two.
//
// It also fed PRINT and PDF EXPORT, which is the artifact that leaves the
// application and cannot be corrected afterwards.
import MarkdownIt from '../../vendor/markdown-it.bundle.mjs'
import footnote from '../../vendor/markdown-it-footnote.bundle.mjs'
import taskLists from '../../vendor/markdown-it-task-lists.bundle.mjs'
import { renderMermaidDiagram } from '../mermaid-renderer.js'
import { normalizeLanguage } from '../highlighter.js'
import { codeFenceIndexByLine } from './fences.js'
import { sanitizeInto } from './sanitize.js'

// html: true because this dialect preserves inline HTML and the fidelity survey
// records it round-tripping byte-identically; a renderer that escaped it would
// show the source of something the editor renders. It is also what makes
// sanitize.js mandatory rather than defensive.
//
// linkify: false deliberately. Turning bare URLs into links is a decision about
// the DOCUMENT, and this renderer's job is to show what the document says, not
// to improve it. The editor does not do it either, so enabling it here would
// make the two surfaces disagree.
const md = new MarkdownIt({ html: true, linkify: false, typographer: false, breaks: false })
  // The dialect is CommonMark PLUS GFM, and markdown-it's core is CommonMark
  // alone. Without these two the renderer disagrees with the editor on exactly
  // two constructs: a task list keeps its literal brackets, and [^1] is read as
  // a shortcut reference LINK, which is correct CommonMark and wrong here.
  .use(footnote)
  .use(taskLists, { enabled: false })

// markdown-it refuses javascript:, vbscript:, file: and most data: URLs itself,
// at PARSE time, by not building the link at all. That is turned off here so
// that sanitize.js is the single authority on which URLs survive.
//
// This is not a relaxation. markdown-it's check is a denylist of four schemes;
// sanitize.js runs an allowlist, so everything markdown-it refuses is still
// refused. What changes is that the refusal happens somewhere that can SEE it:
// a link dissolved during parsing leaves no element, so nothing downstream can
// tell a refused link from a document that never had one, and the decision went
// unrecorded. It also let markdown-it's image rule — which admits only gif,
// png, jpeg and webp data URIs — quietly drop the inline SVG the asset importer
// accepts.
//
// The window between building the HTML and sanitising it is inside one
// synchronous block, on a DETACHED element that is never inserted: scripts
// added via innerHTML do not execute by specification, and an event handler
// attribute is removed before any task could dispatch to it.
md.validateLink = () => true

// Fenced blocks do not become HTML here. A mermaid fence becomes a drawn
// diagram, and a code fence becomes the shell with its language label, copy
// button, language picker and assistant — none of which is expressible as a
// string, because it is event listeners as much as it is markup. So the fence
// rule emits an empty slot, and the real elements are built into the tree after
// parsing.
//
// The marker is minted per render rather than being a constant. With `html:
// true` a document can write any attribute it likes, and a fixed marker is one
// the document can spell too, claiming a slot it never filled.
function slotMarker() {
  return `data-drmd-slot-${Math.random().toString(36).slice(2)}`
}

// Module state, because markdown-it gives a renderer rule no way to reach a
// per-call context. It is safe for exactly one reason: md.render is
// SYNCHRONOUS, so no other render can begin between the assignment below and
// the last rule call it triggers. Everything asynchronous — mermaid — happens
// after render has returned, and touches only local variables.
let collecting = null

md.renderer.rules.fence = (tokens, idx) => {
  const token = tokens[idx]
  collecting.blocks.push({
    source: token.content,
    language: token.info.trim().split(/\s+/)[0] ?? '',
    // The source line is kept rather than a running count. See fences.js.
    line: token.map?.[0] ?? null,
  })
  return `<pre ${collecting.marker}="${collecting.blocks.length - 1}"></pre>\n`
}

// renderMarkdown returns the rendered nodes.
//
// `codeBlock` builds the element for one non-mermaid fence, given
// ({ source, language, fenceIndex }). It is a parameter rather than an import
// because that chrome belongs to the application — clipboard, language picker,
// assistant — and importing it here would close a cycle back through app.js.
// Without one, a fence renders as a plain <pre><code>, which is what a caller
// that only wants HTML should get.
export async function renderMarkdown(markdown, { codeBlock = null, onRefused = null } = {}) {
  const source = String(markdown ?? '')
  const marker = slotMarker()

  collecting = { marker, blocks: [] }
  let html
  let blocks
  try {
    html = md.render(source)
  } finally {
    blocks = collecting.blocks
    collecting = null
  }

  const host = document.createElement('div')
  host.innerHTML = html
  // Before anything reads this tree, and before mermaid or the code chrome is
  // built into it: an opened document is untrusted input, and this DOM shares
  // an origin with the native bindings. See sanitize.js.
  sanitizeInto(host, { keepAttributes: [marker], onRefused })

  // The fence index is looked up by source line rather than counted here,
  // because it is an index INTO THE DOCUMENT that the assistant will rewrite.
  // See codeFenceIndexByLine.
  const fenceIndexes = codeFenceIndexByLine(source)
  await Promise.all(
    Array.from(host.querySelectorAll(`pre[${marker}]`)).map(async (slot) => {
      const block = blocks[Number(slot.getAttribute(marker))]
      slot.removeAttribute(marker)
      if (!block) return
      const fenceIndex = fenceIndexes.get(block.line) ?? null

      if (normalizeLanguage(block.language) === 'mermaid') {
        const diagram = document.createElement('div')
        diagram.className = 'mermaid-render'
        diagram.dataset.language = 'mermaid'
        try {
          diagram.innerHTML = await renderMermaidDiagram(block.source)
        } catch {
          // A diagram that fails to draw must not take the page with it. The
          // print surface especially: a failed render should cost a figure, not
          // the document.
        }
        slot.replaceWith(diagram)
        return
      }

      if (!codeBlock) {
        const code = document.createElement('code')
        code.dataset.language = block.language
        code.textContent = block.source
        slot.append(code)
        return
      }
      slot.replaceWith(codeBlock({ source: block.source, language: block.language, fenceIndex }))
    }),
  )
  return Array.from(host.childNodes)
}
