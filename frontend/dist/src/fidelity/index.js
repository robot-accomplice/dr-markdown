// The fidelity domain: everything this app does to keep a user's markdown
// byte-faithful across a round trip through the vendored editor.
//
// The editor parses markdown into ProseMirror's model and re-serializes the
// WHOLE document, so anything the model cannot express is rewritten. Each
// module here compensates for one such loss. They used to live as ad-hoc
// function pairs on the editor adapter under four different verbs — split,
// protect, collect, collect — with their restore steps hand-ordered in a
// chain. Nothing said they were the same kind of thing, so nobody read them as
// a set, and a footnote definition collected as a link reference definition
// grew users' files by one copy per save before anyone noticed.
//
// TWO PORTS, because there are genuinely two shapes:
//
//   Preservation      runs AFTER the serializer. Captures something from the
//                     original document and puts it back into the output.
//                     { name, capture(markdown) -> { state, markdown },
//                              restore(text, state) -> string }
//                     capture MAY transform the markdown the editor is given
//                     (frontmatter is removed; break tags are substituted).
//                     Returning the input unchanged is normal.
//
//   SerializerPolicy  runs BEFORE the serializer, once per build. Reads the
//                     document and returns options to apply. It never touches
//                     the output, so it has no restore step. Forcing it into
//                     the Preservation port would mean a restore that does
//                     nothing — a shape that lies about what the module is.
//                     { name, detect(markdown) -> object }

import { trailing } from './trailing.js'
import { frontmatter } from './frontmatter.js'
import { breaks } from './breaks.js'
import { linkReferences } from './linkrefs.js'
import { altText } from './alttext.js'
import { markdownStyle } from './style.js'

// CAPTURE order. Each module receives the previous one's markdown, so a module
// that transforms the text affects what the ones after it see.
//
// `trailing` is FIRST because it must read the original document: frontmatter
// splitting would otherwise hand it a body, and a document that is nothing but
// frontmatter has an empty body whose trailing run is not the file's. It
// transforms nothing, so reading first costs the others nothing.
export const PRESERVATIONS = [trailing, frontmatter, breaks, linkReferences, altText]

// RESTORE order, which is NOT the reverse of capture and is load-bearing. It
// reproduces the hand-written chain this registry replaced, verbatim:
// frontmatter is prepended late, and trailing governs the final bytes so it
// must run last — including after the definition block linkrefs appends.
export const RESTORE_SEQUENCE = [linkReferences, altText, breaks, frontmatter, trailing]

export const SERIALIZER_POLICIES = [markdownStyle]

// capturePreservations runs every capture in order, threading the markdown
// through, and returns the text the editor should be given plus the states to
// hand back to restorePreservations.
export function capturePreservations(markdown) {
  const states = new Map()
  let text = markdown
  for (const preservation of PRESERVATIONS) {
    const { state, markdown: next } = preservation.capture(text)
    states.set(preservation.name, state)
    text = next
  }
  return { markdown: text, states }
}

// restorePreservations puts everything back, in restore order.
export function restorePreservations(serialized, states) {
  let text = serialized
  for (const preservation of RESTORE_SEQUENCE) {
    text = preservation.restore(text, states.get(preservation.name))
  }
  return text
}

// detectSerializerOptions merges every policy's reading of the document. One
// policy today; the merge is here so a second cannot silently overwrite it.
export function detectSerializerOptions(markdown) {
  return SERIALIZER_POLICIES.reduce((options, policy) => Object.assign(options, policy.detect(markdown)), {})
}
