// How far one Tab indents, everywhere in the application.
//
// Reported from real use: "indenting is different when tab is pressed in
// raw/split versus wysiwyg". Measured on the same document — a Go fence with
// the caret at the start of the code line:
//
//	raw        Tab -> "    fmt.Println()"   4 spaces
//	formatted  Tab -> "  fmt.Println()"     2 spaces
//
// Raw mode inserted its own constant; the formatted view's code blocks are
// CodeMirror, whose `indentUnit` facet defaults to two spaces. Neither was
// wrong on its own, and nothing connected them, so the same keystroke in the
// same document produced different text depending on which mode you were in.
//
// This module is that connection. Both modes read WIDTH from here, and
// tools/vendor.sh exports CodeMirror's facet so the formatted view can be
// configured from app code rather than by rewriting a value inside a vendored
// bundle — a product decision does not belong in a shell script.
//
// The CSS token `--code-tab-size` in app.css must agree with WIDTH. CSS cannot
// import this, so the agreement is asserted by a test rather than by the
// language.
// Spaces rather than a tab character: the document is markdown, where a literal
// tab at the start of a line has block meaning, and the fidelity survey records
// tab-after-marker being rewritten to a space on round trip.
export const INDENT_WIDTH = 4
export const INDENT = ' '.repeat(INDENT_WIDTH)
