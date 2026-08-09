// WysiwygEditor wraps the vendored Milkdown Crepe bundle.
import { Crepe } from '../vendor/crepe.bundle.mjs'
import { collectLinkReferences, restoreLinkReferences } from './linkrefs.js'

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

// Markdown image syntax: alt, destination, optional title.
//
// The alt alternation admits escapes and one level of balanced brackets. A flat
// `[^\]]*` stopped at the first `]`, so `![alt with [x] inside](b.png)` matched
// nothing at all, was absent from the recorded map, and restoration then wrote
// `![](b.png)` — deleting the caption it exists to protect.
const IMAGE = /!\[((?:\\.|\[[^\]]*\]|[^[\]\\])*)\]\(([^()\s]*)((?:\s+"[^"]*")?)\)/g

// The shape Crepe writes into the alt slot — `ratio.toFixed(2)`. Deliberately
// exact so a real alt text of "3" or "1.5" is left alone.
const RATIO_ALT = /^\d+\.\d{2}$/

// The vendored editor stores its image-resize ratio IN the alt attribute:
// parsing does `Number(alt || 1)` and discards the text, and serializing writes
// `ratio.toFixed(2)` back out. So `![Architecture diagram](arch.png)` returns as
// `![1.00](arch.png)` — the alt text is gone from the file, and gone from the
// document model too, so nothing downstream can recover it.
//
// Alt text is content: it is what a screen reader announces and what shows when
// the image is missing. Editor-private UI state must not be written into a
// public field of the user's file, so the ratio is dropped rather than
// preserved — it means nothing to any other markdown renderer anyway. Recording
// the originals here rather than patching the bundle keeps the fix alive across
// `tools/vendor.sh` refreshes.
// Every alt in the document, in order, grouped by destination. A document may
// reference one asset more than once — a before/after comparison is the obvious
// case — with a different caption each time, so one alt per URL is wrong: the
// last one read would be stamped over every occurrence. Ratio-shaped alts are
// recorded too, because a file that already says `![1.00](x.png)` on disk means
// exactly that, and because excluding them deleted any real alt that happened
// to be a two-decimal number.
function collectAltText(markdown) {
  const byURL = new Map()
  for (const [, alt, url] of markdown.matchAll(IMAGE)) {
    if (!byURL.has(url)) byURL.set(url, [])
    byURL.get(url).push(alt)
  }
  return byURL
}

// Restores in document order, consuming each destination's queue. An image the
// map does not know — one added inside the editor, where Crepe discarded its alt
// before we ever saw it — gets an empty alt rather than the meaningless ratio.
//
// Known limit: deleting one of several images that share a destination shifts
// the remaining captions by one, because the queue is positional and there is
// nothing in the serialized output tying an image back to its original slot.
// That is a narrower and less destructive failure than either the ratio
// overwrite or the last-alt-wins bug it replaces.
function restoreAltText(markdown, byURL) {
  const pending = new Map()
  for (const [url, alts] of byURL) pending.set(url, [...alts])
  return markdown.replace(IMAGE, (whole, alt, url, title) => {
    if (!RATIO_ALT.test(alt)) return whole
    const queue = pending.get(url)
    return `![${queue?.length ? queue.shift() : ''}](${url}${title})`
  })
}

// The exact spellings the vendored editor strips from the parsed document.
// Taken from the bundle, which matches `html` nodes whose trimmed value is one
// of these — case-sensitively, which is why `<BR>` survives untouched.
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
function protectBreaks(markdown) {
  const originals = []
  const protectedMarkdown = markdown.replace(BREAK_LIKE, (match) => {
    if (!STRIPPED_BREAKS.includes(match) && match !== BREAK_SENTINEL) return match
    originals.push(match)
    return BREAK_SENTINEL
  })
  return [protectedMarkdown, originals]
}

function restoreBreaks(markdown, originals) {
  const queue = [...originals]
  return markdown.replaceAll(BREAK_SENTINEL, () => queue.shift() ?? '<br>')
}

export class WysiwygEditor {
  #crepe = null
  #onChange = null
  // The `<br>` spellings removed from the document before the editor saw them.
  #breaks = []
  // Link reference definitions and how each was referenced, restored on exit.
  #linkRefs = null
  // Alt text of every image in the document as opened, keyed by destination.
  #altByURL = new Map()
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
    const [frontmatter, rawBody] = splitFrontmatter(markdown)
    this.#frontmatter = frontmatter
    const [body, breaks] = protectBreaks(rawBody)
    this.#breaks = breaks
    this.#linkRefs = collectLinkReferences(body)
    this.#altByURL = collectAltText(body)
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
    const withRefs = restoreLinkReferences(md, this.#linkRefs)
    return this.#frontmatter + restoreBreaks(restoreAltText(withRefs, this.#altByURL), this.#breaks)
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
