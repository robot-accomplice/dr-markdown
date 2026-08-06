// WysiwygEditor wraps the vendored Milkdown Crepe bundle.
import { Crepe } from '../vendor/crepe.bundle.mjs'

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

export class WysiwygEditor {
  #crepe = null
  #onChange = null

  async create(host, markdown, onChange) {
    this.#onChange = onChange
    await this.#build(host, markdown)
  }

  async #build(host, markdown) {
    host.replaceChildren()
    const crepe = new Crepe({
      root: host,
      defaultValue: markdown,
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
        if (md !== prev) this.#onChange?.(md)
      })
    })
    await crepe.create()
    this.#crepe = crepe
  }

  getMarkdown() {
    return this.#crepe.getMarkdown()
  }

  // Replaces the whole document by rebuilding the editor (see Global
  // Constraints: rebuild strategy). Used on open and on raw→WYSIWYG return.
  async setMarkdown(host, markdown) {
    await this.#crepe.destroy()
    await this.#build(host, markdown)
  }

  async destroy() {
    await this.#crepe?.destroy()
    this.#crepe = null
  }
}
