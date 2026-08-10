// Image tokens in a markdown document, in both forms the product writes.
//
// Sizing is expressed as an `<img src alt width>` tag because CommonMark has no
// size syntax, so a document can hold either form and they are not
// interchangeable — a parser that handles only `![alt](path)` silently drops
// every sized image's width.

export const IMAGE_TOKEN_SOURCE = /!\[[^\]]*\]\([^)\s]+(?:\s+"[^"]*")?\)|<img\b[^>]*>/.source

export function imageTokens(md) {
  const tokens = []
  const pattern = new RegExp(IMAGE_TOKEN_SOURCE, 'g')
  let match = pattern.exec(md)
  while (match) {
    tokens.push({ text: match[0], start: match.index, end: match.index + match[0].length })
    match = pattern.exec(md)
  }
  return tokens
}

export function parseImageToken(text) {
  if (text.startsWith('![')) {
    const parsed = text.match(/^!\[([^\]]*)\]\(([^)\s]+)/)
    return { alt: parsed?.[1] ?? '', path: parsed?.[2] ?? '', width: '' }
  }
  return {
    alt: htmlImageAttribute(text, 'alt'),
    path: htmlImageAttribute(text, 'src'),
    width: htmlImageAttribute(text, 'width'),
  }
}

export function formatImageToken({ alt, path, width }) {
  if (!width) return `![${alt}](${path})`
  return `<img src="${path}" alt="${alt}" width="${width}">`
}

export function selectedImageToken(md, imageIndex) {
  return imageTokens(md)[Number.isInteger(imageIndex) ? imageIndex : 0] ?? null
}

// rewriteImage replaces exactly the selected image. transform returning null
// deletes it, dropping the line when the image was the whole line.
export function rewriteImage(md, imageIndex, transform) {
  const target = selectedImageToken(md, imageIndex)
  if (!target) return md
  const next = transform(parseImageToken(target.text))
  const before = md.slice(0, target.start)
  const after = md.slice(target.end)
  if (next === null) {
    const emptyLine = /(^|\n)$/.test(before) && /^(\n|$)/.test(after)
    return emptyLine ? before + after.replace(/^\n/, '') : before + after
  }
  return before + formatImageToken(next) + after
}

export function htmlImageAttribute(tag, name) {
  const match = tag.match(new RegExp(`\\b${name}\\s*=\\s*"([^"]*)"`, 'i'))
  return match?.[1] ?? ''
}
