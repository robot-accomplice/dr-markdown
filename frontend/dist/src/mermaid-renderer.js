let initialized = false
let renderId = 1

export async function renderMermaidDiagram(source) {
  const mermaid = globalThis.mermaid
  if (!mermaid) return fallbackDiagram('Mermaid runtime unavailable')
  if (!initialized) {
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      theme: 'base',
      themeVariables: {
        primaryColor: '#e2f8f6',
        primaryBorderColor: '#01837f',
        primaryTextColor: '#2c3438',
        lineColor: '#777a80',
        secondaryColor: '#ffffff',
        tertiaryColor: '#f7f7f8',
        noteBkgColor: '#e2f8f6',
        noteBorderColor: '#8bd4ce',
        edgeLabelBackground: '#ffffff',
        fontFamily: 'Public Sans, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif',
      },
    })
    initialized = true
  }
  try {
    const { svg } = await mermaid.render(`dr-mermaid-${renderId++}`, source)
    return svg
  } catch (error) {
    return fallbackDiagram(error?.message || 'Mermaid render failed')
  }
}

function fallbackDiagram(message) {
  return `<div class="mermaid-error">${escapeHtml(message)}</div>`
}

function escapeHtml(text) {
  return String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}
