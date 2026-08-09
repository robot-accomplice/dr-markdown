// Editor commands as document transforms: given a command name, the document
// and what the user has selected, return the new document. No DOM, no editor
// instance — which is what makes every command directly testable.
import {
  tableMarkdown, addTableRow, removeTableRow, addTableColumn, removeTableColumn,
  alignTable, deleteTable,
} from './tables.js'
import { rewriteImage } from './images.js'

export function applyCommand(command, md, editorContext = {}) {
  const selected = editorContext.selectionText?.trim()
  if (selected) return applySelectionCommand(command, md, selected)
  const currentBlock = editorContext.blockText?.trim()
  if (currentBlock) return applyCurrentBlockCommand(command, md, currentBlock)

  const transforms = {
    bold: (text) => appendBlock(text, '**bold text**'),
    italic: (text) => appendBlock(text, '*italic text*'),
    strike: (text) => appendBlock(text, '~~strikethrough text~~'),
    'inline-code': (text) => appendBlock(text, '`code`'),
    highlight: (text) => appendBlock(text, '<mark>highlighted text</mark>'),
    link: (text) => appendBlock(text, '[link text](https://example.com)'),
    math: (text) => appendBlock(text, '$$\nE = mc^2\n$$'),
    'bullet-list': (text) => appendBlock(text, '- List item'),
    'numbered-list': (text) => appendBlock(text, '1. List item'),
    'task-list': (text) => appendBlock(text, '- [ ] Task item'),
    normal: (text) => text,
    h1: (text) => appendBlock(text, '# Heading 1'),
    h2: (text) => appendBlock(text, '## Heading 2'),
    h3: (text) => appendBlock(text, '### Heading 3'),
    h4: (text) => appendBlock(text, '#### Heading 4'),
    h5: (text) => appendBlock(text, '##### Heading 5'),
    h6: (text) => appendBlock(text, '###### Heading 6'),
    quote: (text) => appendBlock(text, '> Quote'),
    indent: (text) => indentLastListItem(text),
    outdent: (text) => outdentLastListItem(text),
    table: (text) => appendBlock(text, tableMarkdown(3, 3)),
    'code-block': (text) => appendBlock(text, '```text\ncode\n```'),
    mermaid: (text) => appendBlock(text, '```mermaid\ngraph TD\n  A[Start] --> B[Finish]\n```'),
    hr: (text) => appendBlock(text, '---'),
    'table-add-row': (text) => addTableRow(text, editorContext.tableIndex),
    'table-remove-row': (text) => removeTableRow(text, editorContext.tableIndex),
    'table-add-column': (text) => addTableColumn(text, editorContext.tableIndex),
    'table-remove-column': (text) => removeTableColumn(text, editorContext.tableIndex),
    'table-align-left': (text) => alignTable(text, 'left', editorContext.tableIndex),
    'table-align-center': (text) => alignTable(text, 'center', editorContext.tableIndex),
    'table-align-right': (text) => alignTable(text, 'right', editorContext.tableIndex),
    'table-delete': (text) => deleteTable(text, editorContext.tableIndex),
    'image-delete': (text) => rewriteImage(text, editorContext.imageIndex, () => null),
  }
  return transforms[command]?.(md) ?? md
}

export function applySelectionCommand(command, md, selected) {
  const inlineTransforms = {
    bold: (text) => `**${text}**`,
    italic: (text) => `*${text}*`,
    strike: (text) => `~~${text}~~`,
    'inline-code': (text) => `\`${text}\``,
    highlight: (text) => `<mark>${text}</mark>`,
    link: (text) => `[${text}](https://example.com)`,
  }
  if (inlineTransforms[command]) return replaceFirstSelection(md, selected, inlineTransforms[command])

  const headingLevel = command.match(/^h([1-6])$/)?.[1]
  if (headingLevel) return formatLineContainingSelection(md, selected, Number(headingLevel))
  if (command === 'normal') return formatLineContainingSelection(md, selected, 0)
  if (command === 'quote') return quoteLineContainingSelection(md, selected)
  if (command === 'code-block') return codeBlockContainingSelection(md, selected)
  if (command === 'bullet-list') return listLineContainingSelection(md, selected, '-')
  if (command === 'numbered-list') return listLineContainingSelection(md, selected, '1.')
  if (command === 'task-list') return listLineContainingSelection(md, selected, '- [ ]')

  return applyCommand(command, md, {})
}

export function applyCurrentBlockCommand(command, md, currentBlock) {
  const headingLevel = command.match(/^h([1-6])$/)?.[1]
  if (headingLevel) return formatLineContainingSelection(md, currentBlock, Number(headingLevel))
  if (command === 'normal') return formatLineContainingSelection(md, currentBlock, 0)
  if (command === 'quote') return quoteLineContainingSelection(md, currentBlock)
  if (command === 'code-block') return codeBlockContainingSelection(md, currentBlock)
  if (command === 'bullet-list') return listLineContainingSelection(md, currentBlock, '-')
  if (command === 'numbered-list') return listLineContainingSelection(md, currentBlock, '1.')
  if (command === 'task-list') return listLineContainingSelection(md, currentBlock, '- [ ]')
  return applyCommand(command, md, {})
}

export function replaceFirstSelection(md, selected, transform) {
  const index = md.indexOf(selected)
  if (index === -1) return md
  return `${md.slice(0, index)}${transform(selected)}${md.slice(index + selected.length)}`
}

export function formatLineContainingSelection(md, selected, level) {
  return rewriteLineContainingSelection(md, selected, (line) => {
    const text = line.replace(/^(\s*>+\s*)?#{1,6}\s+/, '').replace(/^(\s*>+\s*)/, '')
    return level === 0 ? text : `${'#'.repeat(level)} ${text}`
  })
}

export function quoteLineContainingSelection(md, selected) {
  return rewriteLineContainingSelection(md, selected, (line) => {
    const text = line.replace(/^>\s*/, '')
    return `> ${text}`
  })
}

export function listLineContainingSelection(md, selected, marker) {
  return rewriteLineContainingSelection(md, selected, (line) => {
    const text = line.replace(/^\s*(?:[-*+]|\d+\.|- \[[ xX]\])\s+/, '')
    return `${marker} ${text}`
  })
}

export function codeBlockContainingSelection(md, selected) {
  return rewriteLineContainingSelection(md, selected, (line) => `\`\`\`text\n${line}\n\`\`\``)
}

export function rewriteLineContainingSelection(md, selected, transform) {
  const index = md.indexOf(selected)
  if (index === -1) return md
  const lineStart = md.lastIndexOf('\n', index - 1) + 1
  const nextBreak = md.indexOf('\n', index)
  const lineEnd = nextBreak === -1 ? md.length : nextBreak
  return `${md.slice(0, lineStart)}${transform(md.slice(lineStart, lineEnd))}${md.slice(lineEnd)}`
}

export function appendBlock(md, block) {
  const trimmed = md.replace(/\s+$/, '')
  return `${trimmed}${trimmed ? '\n\n' : ''}${block}\n`
}

export function rewriteLastMatchingLine(md, pattern, transform) {
  const lines = md.split('\n')
  for (let i = lines.length - 1; i >= 0; i--) {
    if (pattern.test(lines[i])) {
      lines[i] = transform(lines[i])
      return lines.join('\n')
    }
  }
  return md
}

export function indentLastListItem(md) {
  return rewriteLastMatchingLine(md, /^(\s*)([-*+]|\d+\.)\s+/, (line) => `  ${line}`)
}

export function outdentLastListItem(md) {
  return rewriteLastMatchingLine(md, /^\s{2,}([-*+]|\d+\.)\s+/, (line) => line.replace(/^ {1,2}/, ''))
}
