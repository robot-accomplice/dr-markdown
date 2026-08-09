// YAML frontmatter, only at the very start of the document.
// Trailing blank lines are part of the captured block: the editor trims
// leading blank lines from its body, so leaving the separator behind would
// silently close the gap between frontmatter and the first heading.
const FRONTMATTER = /^---\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n(?:[ \t]*\r?\n)*)?/

// The frontmatter is kept verbatim and never shown to the editor.
//
// Crepe parses `---` as a thematic break and re-serializes the block as `***`
// followed by a setext heading, so a single edit silently destroyed the title,
// date, tags and draft status of every Hugo, Jekyll, Obsidian and Astro
// document. Markdown on disk is this product's source of truth; bytes the
// editor cannot represent must not pass through it.
export const frontmatter = {
  name: 'frontmatter',
  capture(markdown) {
    const match = markdown.match(FRONTMATTER)
    if (!match) return { state: '', markdown }
    return { state: match[0], markdown: markdown.slice(match[0].length) }
  },
  restore: (text, state) => state + text,
}
