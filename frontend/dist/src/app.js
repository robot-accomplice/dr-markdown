// App bootstrap, state, and mode toggle. File wiring arrives in Tasks 6–7.
import { loadTheme, WysiwygEditor } from './editor.js'
import { RawEditor } from './rawmode.js'

const WELCOME = `# Welcome to Dr. Markdown

A **WYSIWYG** markdown editor with *GFM* support.

- Tables, task lists, strikethrough
- Toggle raw mode with Ctrl/Cmd+E

\`\`\`go
fmt.Println("hello")
\`\`\`
`

const state = {
  mode: 'wysiwyg', // 'wysiwyg' | 'raw'
  path: '',
  dirty: false,
  savedText: '',
}

const els = {
  wysiwyg: document.getElementById('wysiwyg'),
  raw: document.getElementById('raw'),
  docTitle: document.getElementById('doc-title'),
  btnToggle: document.getElementById('btn-toggle-mode'),
}

const wysiwyg = new WysiwygEditor()
const raw = new RawEditor()

function getMarkdown() {
  return state.mode === 'raw' ? raw.getMarkdown() : wysiwyg.getMarkdown()
}

async function setMarkdown(md) {
  if (state.mode === 'raw') {
    raw.replaceAll(md)
  } else {
    await wysiwyg.setMarkdown(els.wysiwyg, md)
  }
}

// CommonMark parsing is total — any raw text can become a WYSIWYG doc.
async function toggleMode() {
  if (state.mode === 'wysiwyg') {
    const md = wysiwyg.getMarkdown()
    els.wysiwyg.hidden = true
    els.raw.hidden = false
    raw.open(els.raw, md)
    state.mode = 'raw'
    els.btnToggle.textContent = 'WYSIWYG'
  } else {
    const md = raw.getMarkdown()
    raw.close()
    els.raw.hidden = true
    els.wysiwyg.hidden = false
    await wysiwyg.setMarkdown(els.wysiwyg, md)
    state.mode = 'wysiwyg'
    els.btnToggle.textContent = 'Raw'
  }
}

function onEdited(_md) {
  // Dirty tracking arrives in Task 7.
}

function wire() {
  els.btnToggle.addEventListener('click', toggleMode)
  document.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'e') {
      e.preventDefault()
      toggleMode()
    }
  })
}

async function boot() {
  await loadTheme()
  await wysiwyg.create(els.wysiwyg, WELCOME, onEdited)
  state.savedText = WELCOME
  wire()

  // Test/service hooks used by the chromedp e2e suite.
  window.__app = {
    ready: true,
    state,
    getMarkdown,
    setMarkdown,
    toggleMode,
    debugReplaceRaw: (text) => raw.replaceAll(text),
  }
}

boot()
