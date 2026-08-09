// The document's own trailing newline run.
//
// The serializer emits a blank line after a document that ends in a block — a
// list, a footnote block — so every such file gained a line on first edit.
// Trailing blank lines are not content the editor can hold, so the original
// shape is the only faithful answer; this includes a file that ended with no
// newline at all, which is equally the author's choice to keep.
//
// This also fixes a bug that looked unrelated: a list-first document was marked
// modified the moment it opened, because the re-serialized text differed from
// the file by exactly this one newline, so quitting offered to save text the
// user never wrote.
//
// Captured FIRST, so it reads the original document rather than a body with
// frontmatter already removed.
export const trailing = {
  name: 'trailing',
  capture: (markdown) => ({ state: markdown.match(/\n*$/)[0], markdown }),
  restore: (text, state) => text.replace(/\n*$/, state),
}
