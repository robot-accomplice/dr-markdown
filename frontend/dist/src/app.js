// App bootstrap, state, mode toggle, and file wiring.
import { loadTheme, WysiwygEditor } from './editor.js'
import { RawEditor } from './rawmode.js'
import { bridge } from './bridge.js'

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
  btnOpen: document.getElementById('btn-open'),
  btnSave: document.getElementById('btn-save'),
  btnSaveAs: document.getElementById('btn-save-as'),
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

function refreshTitle() {
  els.docTitle.textContent = (state.path || 'untitled') + (state.dirty ? ' •' : '')
}

async function toggleMode() {
  if (state.mode === 'wysiwyg') {
    const md = wysiwyg.getMarkdown()
    els.wysiwyg.hidden = true
    els.raw.hidden = false
    raw.open(els.raw, md, onEdited)
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

// --- files ---

async function openDocument() {
  if (state.dirty) {
    // Ask Go to resolve the dirty buffer (Save / Don't Save / Cancel);
    // a false answer means Cancel — keep the current document.
    const proceed = await bridge.resolveUnsavedChanges()
    if (!proceed) return
  }
  cancelPendingPush()
  const res = await bridge.openDocument()
  if (!res || (!res.path && !res.content)) return // canceled or unavailable
  await setMarkdown(res.content)
  state.path = res.path
  state.savedText = res.content
  state.dirty = false
  bridge.setDirty(false)
  lastPushedDirty = false
  refreshTitle()
}

async function save() {
  const md = getMarkdown()
  if (!state.path) {
    return saveAs()
  }
  await bridge.saveDocument(state.path, md)
  cancelPendingPush()
  state.savedText = md
  state.dirty = false
  bridge.setDirty(false)
  lastPushedDirty = false
  refreshTitle()
}

async function saveAs() {
  const md = getMarkdown()
  const path = await bridge.saveDocumentAs(md)
  if (!path) return // canceled
  cancelPendingPush()
  state.path = path
  state.savedText = md
  state.dirty = false
  lastPushedDirty = false
  refreshTitle()
}

// --- dirty tracking ---
// The dirty flag is pushed to Go immediately when it changes (so the
// native close guard and title are always current); only the full content
// push is debounced — every keystroke would be too chatty over the bridge.
// 300ms after the last edit, Go gets the latest content (for the
// close-guard save path).
let pushTimer = null
let lastPushedDirty = null

function onEdited(md) {
  state.dirty = md !== state.savedText
  refreshTitle()
  if (state.dirty !== lastPushedDirty) {
    bridge.setDirty(state.dirty)
    lastPushedDirty = state.dirty
  }
  clearTimeout(pushTimer)
  pushTimer = setTimeout(() => {
    bridge.updateContent(md)
  }, 300)
}

// cancelPendingPush drops any debounced content push; called before the
// document is replaced (open/save) so a stale push can't overwrite fresh
// state in Go.
function cancelPendingPush() {
  clearTimeout(pushTimer)
  pushTimer = null
}

function wire() {
  els.btnToggle.addEventListener('click', toggleMode)
  els.btnOpen.addEventListener('click', openDocument)
  els.btnSave.addEventListener('click', save)
  els.btnSaveAs.addEventListener('click', saveAs)
  document.addEventListener('keydown', (e) => {
    const mod = e.ctrlKey || e.metaKey
    if (!mod) return
    const key = e.key.toLowerCase()
    if (key === 'e') {
      e.preventDefault()
      toggleMode()
    } else if (key === 'o') {
      e.preventDefault()
      openDocument()
    } else if (key === 's' && e.shiftKey) {
      e.preventDefault()
      saveAs()
    } else if (key === 's') {
      e.preventDefault()
      save()
    }
  })
}

async function boot() {
  await loadTheme()
  await wysiwyg.create(els.wysiwyg, WELCOME, onEdited)
  state.savedText = WELCOME
  wire()
  refreshTitle()

  // Test/service hooks used by the chromedp e2e suite.
  window.__app = {
    ready: true,
    state,
    getMarkdown,
    setMarkdown,
    toggleMode,
    openDocument,
    save,
    saveAs,
    debugReplaceRaw: (text) => raw.replaceAll(text),
    debugSimulateEdit: (md) => onEdited(md),
  }
}

boot()
