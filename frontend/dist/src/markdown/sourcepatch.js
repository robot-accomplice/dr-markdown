// Source-preserving patching: keep the user's original bytes for everything an
// edit did not touch.
//
// PROBE CODE. Not wired into the app. See
// docs/superpowers/specs/2026-08-10-source-range-patching-design.md.
//
// The premise, measured before this file was written: the respelling the editor
// does lives in the SERIALIZER, not in the tree. `| --- | --- |` and `| - | - |`
// parse to the same table node, because a delimiter row is alignment metadata
// rather than content. So two trees that compare equal can be represented by
// the ORIGINAL bytes, and the respelling never reaches the file.

// structuralEquals compares two mdast values ignoring `position` at every depth.
//
// Position is exactly what must be ignored: the same construct at a different
// offset is the same construct, and that is the whole basis for keeping its
// original bytes.
export function structuralEquals(a, b) {
  if (a === b) return true
  if (a === null || b === null) return false
  if (typeof a !== 'object' || typeof b !== 'object') return false

  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false
    return a.every((item, i) => structuralEquals(item, b[i]))
  }

  const keys = (node) => Object.keys(node).filter((k) => k !== 'position').sort()
  const ka = keys(a)
  const kb = keys(b)
  if (ka.length !== kb.length) return false
  return ka.every((key, i) => key === kb[i] && structuralEquals(a[key], b[key]))
}

// align pairs two lists of top-level nodes by longest common subsequence over
// structural equality.
//
// LCS rather than index-by-index comparison: inserting one block at the top
// would otherwise shift every following block into a "changed" region, and
// re-splicing an untouched table from the editor's output is precisely the
// respelling this design exists to prevent. LCS keeps the blast radius of an
// edit at the blocks the edit actually reached.
//
// Returns ops in document order:
//   { type: 'match',  a, b }              structurally identical
//   { type: 'change', as: [], bs: [] }    a maximal run that differs
export function align(as, bs) {
  const n = as.length
  const m = bs.length

  // lcs[i][j] = length of the longest common subsequence of as[i..] and bs[j..].
  const lcs = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i][j] = structuralEquals(as[i], bs[j])
        ? lcs[i + 1][j + 1] + 1
        : Math.max(lcs[i + 1][j], lcs[i][j + 1])
    }
  }

  const ops = []
  let pendingAs = []
  let pendingBs = []
  const flush = () => {
    if (pendingAs.length || pendingBs.length) {
      ops.push({ type: 'change', as: pendingAs, bs: pendingBs })
      pendingAs = []
      pendingBs = []
    }
  }

  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (structuralEquals(as[i], bs[j])) {
      flush()
      ops.push({ type: 'match', a: as[i], b: bs[j] })
      i++
      j++
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      pendingAs.push(as[i++])
    } else {
      pendingBs.push(bs[j++])
    }
  }
  while (i < n) pendingAs.push(as[i++])
  while (j < m) pendingBs.push(bs[j++])
  flush()

  return ops
}

// patchPreservingSource returns `original` with only the regions an edit
// touched replaced by the editor's serialization of them.
//
// It SPLICES the original rather than rebuilding it from pieces. Rebuilding
// would mean reproducing every blank line and every trailing space between
// blocks; splicing preserves them by construction, because those bytes are
// never written. That is the difference between this and the six fidelity
// modules, which restore what the serializer already destroyed.
//
// `remark` is injected rather than reached for, so this module stays pure and
// can move to Go without carrying an editor dependency with it.
//
// NEVER THROWS. Every failure returns `edited` — today's behaviour. The design
// floor is "never worse than the current editor", and it matters more here than
// usual: this code's failure mode is writing wrong bytes into a user's file.
export function patchPreservingSource(original, edited, remark) {
  let a
  let b
  try {
    a = remark.parse(original)
    b = remark.parse(edited)
  } catch {
    return edited
  }
  if (!a?.children || !b?.children) return edited

  let out = ''
  let cursor = 0

  for (const op of align(a.children, b.children)) {
    if (op.type === 'match') {
      const end = offsetEnd(op.a)
      if (end === null) return edited
      // Everything from the cursor to this node's end, INCLUDING the gap before
      // it, comes from the original. The gap is why blank-line runs survive.
      out += original.slice(cursor, end)
      cursor = end
      continue
    }

    // A changed run with nothing to align against on one side has no
    // unambiguous anchor, and the separator between blocks would have to be
    // guessed. Bail to the floor rather than guess at a user's document;
    // in-place edits are what this probe set out to prove.
    if (op.as.length === 0 || op.bs.length === 0) return edited

    const from = offsetStart(op.as[0])
    const to = offsetEnd(op.as[op.as.length - 1])
    const editedFrom = offsetStart(op.bs[0])
    const editedTo = offsetEnd(op.bs[op.bs.length - 1])
    if (from === null || to === null || editedFrom === null || editedTo === null) return edited

    out += original.slice(cursor, from)
    out += edited.slice(editedFrom, editedTo)
    cursor = to
  }

  // Trailing bytes no node covers — the document's own final newline among them.
  return out + original.slice(cursor)
}

function offsetStart(node) {
  const offset = node?.position?.start?.offset
  return typeof offset === 'number' ? offset : null
}

function offsetEnd(node) {
  const offset = node?.position?.end?.offset
  return typeof offset === 'number' ? offset : null
}
