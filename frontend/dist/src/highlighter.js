const languageAliases = {
  js: 'javascript',
  jsx: 'javascript',
  ts: 'typescript',
  tsx: 'typescript',
  py: 'python',
  rb: 'ruby',
  sh: 'bash',
  shell: 'bash',
  zsh: 'bash',
  yml: 'yaml',
  md: 'markdown',
}

export function highlightMarkdownSource(markdown) {
  return renderMarkdownSource(markdown)
}

export function highlightCode(code, language = '') {
  return highlight(code, language)
}

export function normalizeLanguage(language = '') {
  const key = String(language).trim().toLowerCase()
  return languageAliases[key] || key
}

function highlight(code, language) {
  const hljs = globalThis.hljs
  const normalized = normalizeLanguage(language)
  if (hljs && normalized && hljs.getLanguage(normalized)) {
    return hljs.highlight(code, { language: normalized, ignoreIllegals: true }).value
  }
  if (hljs && !normalized) {
    return hljs.highlightAuto(code).value
  }
  return escapeHtml(code)
}

function renderMarkdownSource(markdown) {
  return String(markdown).split('\n').map(renderMarkdownLine).join('\n')
}

function renderMarkdownLine(line) {
  const fence = line.match(/^(```+)(.*)$/)
  if (fence) {
    return `<span class="hljs-meta">${marker(fence[1])}${escapeHtml(fence[2])}</span>`
  }
  const heading = line.match(/^(#{1,6})(\s+)(.*)$/)
  if (heading) {
    return `<span class="hljs-section">${marker(heading[1])}${marker(heading[2])}${renderMarkdownInline(heading[3])}</span>`
  }
  const quote = line.match(/^(>\s?)(.*)$/)
  if (quote) {
    return `<span class="hljs-quote">${marker(quote[1])}${renderMarkdownInline(quote[2])}</span>`
  }
  const list = line.match(/^(\s*)([-*+]|\d+\.|- \[[ xX]\])(\s+)(.*)$/)
  if (list) {
    return `${escapeHtml(list[1])}<span class="hljs-bullet">${marker(list[2])}${marker(list[3])}</span>${renderMarkdownInline(list[4])}`
  }
  return renderMarkdownInline(line)
}

function renderMarkdownInline(line) {
  let html = escapeHtml(line)
  html = html.replace(/(`)([^`]+)(`)/g, (_match, open, text, close) =>
    `${marker(open)}<span class="hljs-code">${text}</span>${marker(close)}`
  )
  html = html.replace(/(\*\*)([^*]+)(\*\*)/g, (_match, open, text, close) =>
    `${marker(open)}<span class="hljs-strong">${text}</span>${marker(close)}`
  )
  html = html.replace(/(\*)([^*]+)(\*)/g, (_match, open, text, close) =>
    `${marker(open)}<span class="hljs-emphasis">${text}</span>${marker(close)}`
  )
  html = html.replace(/(\[)([^\]]+)(\]\()([^)]+)(\))/g, (_match, open, text, middle, url, close) =>
    `${marker(open)}<span class="hljs-link">${text}</span>${marker(middle)}<span class="hljs-link">${url}</span>${marker(close)}`
  )
  return html
}

function marker(text) {
  return `<span class="markdown-marker">${escapeHtml(text)}</span>`
}

export function escapeHtml(text) {
  return String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}
