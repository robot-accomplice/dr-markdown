// GFM table geometry: locating a table in a document and rewriting its shape.
//
// Every operation is a no-op when the document has no table at the requested
// index, because the command dispatcher can reach these with the cursor
// anywhere — `tableBounds` returning null is the normal case, not an error.

export function tableMarkdown(cols, rows) {
  const header = Array.from({ length: cols }, (_, i) => `Header ${i + 1}`)
  const divider = Array.from({ length: cols }, () => '---')
  const body = Array.from({ length: rows - 1 }, (_, r) =>
    Array.from({ length: cols }, (_, c) => `Cell ${r + 1}.${c + 1}`)
  )
  return [header, divider, ...body].map(tableRow).join('\n')
}

export function tableRow(cells) {
  return `| ${cells.join(' | ')} |`
}

export function splitTableRow(row) {
  return row.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((cell) => cell.trim())
}

export function isTableRow(line) {
  return /^\s*\|.+\|\s*$/.test(line)
}

export function isDividerRow(line) {
  return /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$/.test(line)
}

export function tableBounds(md, tableIndex = 0) {
  const lines = md.split('\n')
  let currentTable = -1
  for (let i = 0; i < lines.length - 1; i++) {
    if (isTableRow(lines[i]) && isDividerRow(lines[i + 1])) {
      currentTable++
      let end = i + 2
      while (end < lines.length && isTableRow(lines[end])) end++
      if (currentTable === (Number.isInteger(tableIndex) ? tableIndex : 0)) return { lines, start: i, end }
      i = end - 1
    }
  }
  return null
}

export function containsTable(md) {
  return tableBounds(md) !== null
}

export function rewriteTable(md, tableIndex, transform) {
  const bounds = tableBounds(md, tableIndex)
  if (!bounds) return md
  const rows = bounds.lines.slice(bounds.start, bounds.end)
  bounds.lines.splice(bounds.start, rows.length, ...transform(rows))
  return bounds.lines.join('\n')
}

export function addTableRow(md, tableIndex = 0) {
  return rewriteTable(md, tableIndex, (rows) => {
    const cols = splitTableRow(rows[0]).length
    rows.push(tableRow(Array.from({ length: cols }, () => '')))
    return rows
  })
}

export function removeTableRow(md, tableIndex = 0) {
  return rewriteTable(md, tableIndex, (rows) => {
    if (rows.length > 3) rows.pop()
    return rows
  })
}

export function addTableColumn(md, tableIndex = 0) {
  return rewriteTable(md, tableIndex, (rows) => rows.map((row, index) => {
    const cells = splitTableRow(row)
    cells.push(index === 0 ? `Header ${cells.length + 1}` : index === 1 ? '---' : '')
    return tableRow(cells)
  }))
}

export function removeTableColumn(md, tableIndex = 0) {
  return rewriteTable(md, tableIndex, (rows) => rows.map((row) => {
    const cells = splitTableRow(row)
    if (cells.length > 1) cells.pop()
    return tableRow(cells)
  }))
}

export function alignTable(md, alignment, tableIndex = 0) {
  const marker = { left: ':---', center: ':---:', right: '---:' }[alignment]
  return rewriteTable(md, tableIndex, (rows) => {
    const cols = splitTableRow(rows[0]).length
    rows[1] = tableRow(Array.from({ length: cols }, () => marker))
    return rows
  })
}

export function deleteTable(md, tableIndex = 0) {
  const bounds = tableBounds(md, tableIndex)
  if (!bounds) return md
  bounds.lines.splice(bounds.start, bounds.end - bounds.start)
  return bounds.lines.join('\n').replace(/\n{3,}/g, '\n\n')
}
