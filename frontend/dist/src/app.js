// App bootstrap and state. File/mode wiring arrives in Tasks 5–7.
import { loadTheme, WysiwygEditor } from './editor.js'

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
}

const wysiwyg = new WysiwygEditor()

function getMarkdown() {
  // Raw mode arrives in Task 5; WYSIWYG is the only source for now.
  return wysiwyg.getMarkdown()
}

async function setMarkdown(md) {
  await wysiwyg.setMarkdown(els.wysiwyg, md)
}

function onEdited(_md) {
  // Dirty tracking arrives in Task 7.
}

async function boot() {
  await loadTheme()
  await wysiwyg.create(els.wysiwyg, WELCOME, onEdited)
  state.savedText = WELCOME

  // Test/service hooks used by the chromedp e2e suite.
  window.__app = {
    ready: true,
    state,
    getMarkdown,
    setMarkdown,
  }
}

boot()
