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

// YAML frontmatter, only at the very start of the document.
// Trailing blank lines are part of the captured block: the editor trims
// leading blank lines from its body, so leaving the separator behind would
// silently close the gap between frontmatter and the first heading.
const FRONTMATTER = /^---\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n(?:[ \t]*\r?\n)*)?/

// splitFrontmatter returns [frontmatter, body]. The frontmatter is kept
// verbatim and never shown to the editor.
//
// Crepe parses `---` as a thematic break and re-serializes the block as `***`
// followed by a setext heading, so a single edit silently destroyed the title,
// date, tags and draft status of every Hugo, Jekyll, Obsidian and Astro
// document. Markdown on disk is this product's source of truth; bytes the
// editor cannot represent must not pass through it.
export function splitFrontmatter(markdown) {
  const match = markdown.match(FRONTMATTER)
  return match ? [match[0], markdown.slice(match[0].length)] : ['', markdown]
}

export class WysiwygEditor {
  #crepe = null
  #onChange = null
  // Frontmatter stripped from the current document, re-attached on the way out.
  #frontmatter = ''
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
    const [frontmatter, body] = splitFrontmatter(markdown)
    this.#frontmatter = frontmatter
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
        this.#onChange?.(this.#frontmatter + md)
      })
    })
    await crepe.create()
    this.#crepe = crepe
    this.#baseline = crepe.getMarkdown()
  }

  getMarkdown() {
    return this.#frontmatter + this.#crepe.getMarkdown()
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
