// WysiwygEditor wraps the vendored Milkdown Crepe bundle.
import { Crepe } from '../vendor/crepe.bundle.mjs'
import { detectMarkdownStyle } from './mdstyle.js'
import { bridge } from './bridge.js'
import { trailing } from './fidelity/trailing.js'
import { breaks } from './fidelity/breaks.js'
import { frontmatter } from './fidelity/frontmatter.js'
import { altText } from './fidelity/alttext.js'
import { linkReferences } from './fidelity/linkrefs.js'

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

// applyMarkdownStyle makes the serializer write the document's own style back.
//
// The options object must be MUTATED, not replaced. `ctx.set` swaps the slice
// value, but the serializer captured a reference to the original object when it
// was built, so a replacement is simply never read — which is why setting the
// slice, before or after create, changed nothing. Assigning onto the object the
// serializer already holds is what takes effect.
//
// Every key is written on every build, using the serializer's own defaults
// where the document expressed no preference. Applying only the detected keys
// would let a style set for one document persist into the next if the options
// object were ever shared.
const STYLE_DEFAULTS = {
  bullet: '*',
  bulletOrdered: '.',
  rule: '*',
  fence: '`',
  setext: false,
  closeAtx: false,
  incrementListMarker: true,
}

function applyMarkdownStyle(crepe, markdown) {
  try {
    const options = crepe.editor?.ctx?.get('remarkStringifyOptions')
    if (!options) return
    Object.assign(options, STYLE_DEFAULTS, detectMarkdownStyle(markdown))
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
  // State captured by the <br> preservation. See fidelity/breaks.js.
  #breakState = []
  // State captured by the link-reference preservation. See fidelity/linkrefs.js.
  #linkRefState = null
  // State captured by the alt-text preservation. See fidelity/alttext.js.
  #altState = new Map()
  // State captured by the frontmatter preservation. See fidelity/frontmatter.js.
  #frontmatterState = ''
  // State captured by the trailing-newline preservation. See fidelity/trailing.js.
  #trailingState = ''
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
    const splitOff = frontmatter.capture(markdown)
    this.#frontmatterState = splitOff.state
    const rawBody = splitOff.markdown
    this.#trailingState = trailing.capture(markdown).state
    const captured = breaks.capture(rawBody)
    this.#breakState = captured.state
    const body = captured.markdown
    this.#linkRefState = linkReferences.capture(body).state
    this.#altState = altText.capture(body).state
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
    applyMarkdownStyle(crepe, body)
    this.#crepe = crepe
    this.#baseline = crepe.getMarkdown()
  }

  getMarkdown() {
    return this.#serialize(this.#crepe.getMarkdown())
  }

  // The single exit for markdown leaving the editor: re-attach the frontmatter
  // the editor never saw, and undo the alt-slot overwrite. Applied on the way
  // out only — #baseline still compares raw Crepe output, so change detection
  // is unaffected.
  #serialize(md) {
    const withRefs = linkReferences.restore(md, this.#linkRefState)
    const body = breaks.restore(altText.restore(withRefs, this.#altState), this.#breakState)
    // Applied last, so it governs the bytes that actually leave — including the
    // definition block restoreLinkReferences appends after the body.
    return trailing.restore(frontmatter.restore(body, this.#frontmatterState), this.#trailingState)
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
