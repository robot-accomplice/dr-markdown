// WysiwygEditor wraps the vendored Milkdown Crepe bundle.
import { Crepe, indentUnit } from '../vendor/crepe.bundle.mjs'
import { bridge } from './bridge.js'
import { capturePreservations, restorePreservations, detectSerializerOptions } from './fidelity/index.js'
import { renderMermaidDiagram } from './mermaid-renderer.js'
import { normalizeLanguage } from './highlighter.js'
import { mermaidFenceSources } from './markdown/fences.js'
import { INDENT } from './indent.js'

// Crepe's code-mirror node view will show a rendered preview in place of a
// block's source whenever renderPreview returns an element, and offers a toggle
// back to the editable source. Mermaid is the one construct in this dialect
// that wants that: a diagram is the point of the fence, but the source behind
// it still has to be editable in place, because WYSIWYG is what this editor is
// for.
//
// The app used to draw these diagrams itself, by replacing the editor's own
// <pre>. That produced a diagram nobody could edit without leaving Formatted
// mode, and it raced the node view's own mounting — whichever ran second won.
// Letting the node view own its DOM removes both problems.
//
// Rendering a diagram is asynchronous and this hook is not, so the node view
// hands it a third argument: a callback that supplies the preview later. That
// matters because the node view does not mount the element it is given, it
// mounts a COPY — measured, by watching a render settle with the returned
// element's `isConnected` false while a diagram sat on screen. Filling in the
// returned element afterwards writes into a detached node.
//
// Returning undefined puts the block in its loading state until the callback
// arrives; returning null means "no preview, show the source", which is the
// answer for every language except mermaid.
// The block language picker's normalizer, reachable from the vendored bundle.
//
// The node view writes whatever the picker hands it into the fence, and the
// picker supplies CodeMirror's DISPLAY name — so it wrote ```Python where every
// other route in this app writes ```python (#78). tools/vendor.sh routes that
// one assignment through here, so what "normalized" means stays in app code
// beside every other user of it rather than being spelled out in a shell script.
//
// It is deliberately NOT applied on serialize: that would rewrite fences the
// user authored capitalised, and a document must come back as it went in.
globalThis.drmd = globalThis.drmd || {}
globalThis.drmd.normalizeLanguage = normalizeLanguage

const mermaidDiagrams = new Map()

function renderCodeBlockPreview(language, content, update) {
  if (normalizeLanguage(language) !== 'mermaid') return null

  const cached = mermaidDiagrams.get(content)
  if (cached !== undefined) return diagramElement(cached)

  renderMermaidDiagram(content)
    .then((svg) => {
      mermaidDiagrams.set(content, svg)
      update(diagramElement(svg))
    })
    .catch((error) => {
      // A diagram that fails to draw must not take the block with it: the
      // source stays reachable through the toggle either way.
      update(diagramElement('<div class="mermaid-error">Mermaid render failed</div>'))
      bridge.recordEvent('editor.mermaid-preview-failed', { error: String(error?.message ?? error) })
    })
  return undefined
}

function diagramElement(html) {
  const host = document.createElement('div')
  host.className = 'mermaid-render'
  host.dataset.language = 'mermaid'
  host.innerHTML = html
  host.append(diagramEditButton())
  return host
}

// The way back to a diagram's assistant, on the diagram itself.
//
// This used to live in a floating bar parked over the top of the document,
// which appeared whenever the document CONTAINED a diagram — caret nowhere near
// it — and was removed for that reason. A per-block action belongs on its
// block.
//
// It is safe to put here, and only here, because this element is the app's own:
// the node view asks for a preview and mounts a copy of whatever it is handed,
// so anything inside it survives a re-render. Appending to the node view's own
// `.tools` group would not — that DOM belongs to the editor and is rebuilt.
//
// It carries NO listener of its own, and that is not an oversight: the node
// view mounts a COPY of this element, and cloning a node does not clone its
// event listeners. A handler attached here would exist on the element that was
// handed over and be absent from the one on screen — the button would render
// and do nothing, which is the exact shape of the defect (#75) that cost this
// project a day. The shell delegates from #wysiwyg instead, and finds this
// button by its data attribute.
function diagramEditButton() {
  const button = document.createElement('button')
  button.type = 'button'
  button.className = 'diagram-edit'
  button.dataset.diagramEdit = 'true'
  button.textContent = 'Edit diagram'
  button.title = 'Edit this diagram'
  return button
}

// Draw every diagram in the document before the editor mounts, so the preview
// hook above is a cache hit and the diagram is present in the very first frame
// the block is painted.
//
// The pass this replaced awaited each render before putting the diagram on
// screen, so diagrams appeared complete or not at all. Filling them in
// afterwards would have been a visible regression — an empty box that pops —
// and it made three existing tests sample the DOM before the diagram existed.
async function primeMermaidDiagrams(markdown) {
  await Promise.all(
    mermaidFenceSources(markdown)
      .filter((source) => !mermaidDiagrams.has(source))
      .map(async (source) => {
        try {
          mermaidDiagrams.set(source, await renderMermaidDiagram(source))
        } catch {
          // Leave it uncached: the preview hook renders it again and reports
          // the failure there rather than holding up the whole document.
        }
      }),
  )
}

// Loads the Crepe theme CSS listed in vendor/theme/manifest.txt.
// Light theme only in this milestone; dark themes land in the polish
// milestone. Skips files that don't exist rather than failing boot.
export async function loadTheme() {
  const res = await fetch('vendor/theme/manifest.txt')
  const files = (await res.text()).trim().split('\n').filter(Boolean)
  for (const f of files) {
    if (f.includes('-dark/')) continue
    if (!f.startsWith('common/') && f !== 'crepe/style.css') continue
    const link = document.createElement('link')
    link.rel = 'stylesheet'
    link.href = `vendor/theme/${f}`
    document.head.appendChild(link)
  }
}

// applySerializerOptions makes the serializer write the document's own style
// back, from whatever the registered policies detect.
//
// The options object must be MUTATED, not replaced. `ctx.set` swaps the slice
// value, but the serializer captured a reference to the original object when it
// was built, so a replacement is simply never read — which is why setting the
// slice, before or after create, changed nothing. Assigning onto the object the
// serializer already holds is what takes effect.
function applySerializerOptions(crepe, markdown) {
  try {
    const options = crepe.editor?.ctx?.get('remarkStringifyOptions')
    if (!options) return
    Object.assign(options, detectSerializerOptions(markdown))
  } catch (error) {
    // A style mismatch is a cosmetic diff; letting it break the editor would
    // trade a formatting nuisance for an unusable document.
    //
    // The catch is right; logging only to console was not. This exact site
    // swallowed a ReferenceError for two rounds of debugging, because a
    // production build has no devtools to read it in. Contain the failure,
    // but never make it invisible.
    console.warn('editor: could not apply document markdown style', error)
    bridge.recordEvent('editor.style-failed', { error: String(error?.message ?? error) })
  }
}

export class WysiwygEditor {
  #crepe = null
  #onChange = null
  // Everything the fidelity registry captured from the document as opened,
  // keyed by preservation name. The editor does not know what is in here.
  #states = new Map()
  // Last serialized markdown we treated as the unedited baseline. Crepe
  // fires an initial markdownUpdated when its async feature mounting
  // finishes — a normalization pass, not a user edit — so events that
  // serialize back to the baseline are ignored.
  #baseline = ''

  async create(host, markdown, onChange) {
    this.#onChange = onChange
    await this.#build(host, markdown)
  }

  async #build(host, markdown) {
    const captured = capturePreservations(markdown)
    this.#states = captured.states
    const body = captured.markdown
    await primeMermaidDiagrams(body)
    host.replaceChildren()
    const crepe = new Crepe({
      root: host,
      defaultValue: body,
      features: {
        // Math is out of dialect (GFM + mermaid only).
        [Crepe.Feature.Latex]: false,
      },
      featureConfigs: {
        [Crepe.Feature.Placeholder]: { text: 'Start writing…', mode: 'block' },
        [Crepe.Feature.CodeMirror]: {
          // Tab indents by the same amount here as it does in raw mode.
          // CodeMirror's own default is two spaces and raw mode's is four, so
          // without this the same keystroke on the same document produced
          // different text depending on the mode. The facet is exported by
          // tools/vendor.sh precisely so this decision lives here.
          extensions: [indentUnit.of(INDENT)],
          renderPreview: renderCodeBlockPreview,
          // Only blocks that produce a preview are affected, and mermaid is the
          // only one that does — so a diagram opens as a diagram, and every
          // other language opens as editable source.
          previewOnlyByDefault: true,
        },
      },
    })
    crepe.on((listener) => {
      listener.markdownUpdated((_ctx, md, prev) => {
        if (md === prev || md === this.#baseline) return
        this.#baseline = md
        this.#onChange?.(this.#serialize(md))
      })
    })
    await crepe.create()
    applySerializerOptions(crepe, body)
    this.#crepe = crepe
    this.#baseline = crepe.getMarkdown()
  }

  getMarkdown() {
    return this.#serialize(this.#crepe.getMarkdown())
  }

  // The single exit for markdown leaving the editor. Applied on the way out
  // only — #baseline still compares raw Crepe output, so change detection is
  // unaffected.
  #serialize(md) {
    return restorePreservations(md, this.#states)
  }

  // Replaces the whole document by rebuilding the editor (see Global
  // Constraints: rebuild strategy). Used on open and on raw→WYSIWYG return.
  async setMarkdown(host, markdown) {
    // Crepe's destroy can race its async feature mounting and throw on a
    // half-mounted feature; the rebuild below clears the host DOM anyway.
    await this.#crepe.destroy().catch(() => {})
    await this.#build(host, markdown)
  }

  async destroy() {
    await this.#crepe?.destroy()
    this.#crepe = null
  }
}
