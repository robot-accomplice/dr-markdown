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
  return highlight(markdown, 'markdown')
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

export function escapeHtml(text) {
  return String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}
