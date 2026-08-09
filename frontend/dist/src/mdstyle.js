// Detects the markdown style a document is written in, so the serializer can
// write it back the same way instead of imposing one house style.
//
// The serializer has fixed defaults: `*` bullets, `.` after ordered numbers,
// ATX headings, backtick fences, `***` thematic breaks. A document written with
// `-` bullets and `~~~` fences came back restyled from end to end after a single
// edit — not data loss, but a whole-file diff for anyone keeping notes in
// version control, and a house style the author never chose.
//
// These options are per-document, so a document with MIXED styles cannot be
// fully preserved: one bullet character has to win. Detection picks the
// most-used, which keeps the majority of lines unchanged and confines the
// rewrite to the minority the author was already inconsistent about. That is a
// real limit and it is the reason this is a reduction rather than a fix.

// Counts occurrences of each candidate at the start of a line, ignoring fenced
// code so a bullet inside an example does not vote on the document's style.
function countLineStarts(markdown, patterns) {
  const counts = new Map(patterns.map((p) => [p.key, 0]))
  let inFence = false
  let fenceMarker = ''
  for (const line of markdown.split('\n')) {
    const fence = line.match(/^ {0,3}(`{3,}|~{3,})/)
    if (fence) {
      const marker = fence[1][0]
      if (!inFence) {
        inFence = true
        fenceMarker = marker
      } else if (marker === fenceMarker) {
        inFence = false
      }
      continue
    }
    if (inFence) continue
    for (const pattern of patterns) {
      if (pattern.test.test(line)) counts.set(pattern.key, counts.get(pattern.key) + 1)
    }
  }
  return counts
}

// Returns the key with the highest count, or null when nothing matched, so the
// caller can leave the serializer's default alone rather than assert a style the
// document never expressed.
function dominant(counts) {
  let best = null
  let bestCount = 0
  for (const [key, count] of counts) {
    if (count > bestCount) {
      best = key
      bestCount = count
    }
  }
  return best
}

// countOpeningFences counts the marker each fenced block OPENS with, ignoring
// closing fences and any fence-like line inside another block.
function countOpeningFences(markdown) {
  const counts = new Map([
    ['`', 0],
    ['~', 0],
  ])
  let openMarker = ''
  for (const line of markdown.split('\n')) {
    const match = line.match(/^ {0,3}(`{3,}|~{3,})/)
    if (!match) continue
    const marker = match[1][0]
    if (!openMarker) {
      openMarker = marker
      counts.set(marker, counts.get(marker) + 1)
    } else if (marker === openMarker) {
      openMarker = ''
    }
  }
  return counts
}

// detectMarkdownStyle returns serializer options matching the document, with
// keys omitted where the document showed no preference.
export function detectMarkdownStyle(markdown) {
  const options = {}

  const bullet = dominant(
    countLineStarts(markdown, [
      { key: '-', test: /^ {0,3}- +\S/ },
      { key: '*', test: /^ {0,3}\* +\S/ },
      { key: '+', test: /^ {0,3}\+ +\S/ },
    ]),
  )
  if (bullet) options.bullet = bullet

  const bulletOrdered = dominant(
    countLineStarts(markdown, [
      { key: '.', test: /^ {0,3}\d+\. +\S/ },
      { key: ')', test: /^ {0,3}\d+\) +\S/ },
    ]),
  )
  if (bulletOrdered) options.bulletOrdered = bulletOrdered

  const rule = dominant(
    countLineStarts(markdown, [
      { key: '-', test: /^ {0,3}-{3,}\s*$/ },
      { key: '*', test: /^ {0,3}\*{3,}\s*$/ },
      { key: '_', test: /^ {0,3}_{3,}\s*$/ },
    ]),
  )
  // A `---` rule is indistinguishable from a setext underline by line shape
  // alone; frontmatter is already removed before this runs, so the remaining
  // risk is a document using setext headings, handled below.
  if (rule) options.rule = rule

  // Fences need their own scan: countLineStarts treats a fence line as a
  // delimiter and skips it, so asking it to count fence markers returned
  // nothing and every `~~~` document came back with backticks.
  const fence = dominant(countOpeningFences(markdown))
  if (fence) options.fence = fence

  // Setext headings only exist for levels 1 and 2, so this is on or off for the
  // whole document rather than a per-heading choice.
  if (/^[^\n]+\n {0,3}(=+|-+)\s*$/m.test(markdown)) options.setext = true

  return options
}
