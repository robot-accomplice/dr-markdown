// App bootstrap, file rail, ribbon commands, mode toggle, and file wiring.
import { loadTheme, WysiwygEditor } from './editor.js'
import { RawEditor } from './rawmode.js'
import { bridge } from './bridge.js'
import { escapeHtml, highlightCode, highlightMarkdownSource, normalizeLanguage } from './highlighter.js'
import { renderMermaidDiagram } from './mermaid-renderer.js'
import {
  firstCodeFenceDescriptor, rewriteCodeFenceLanguage, rewriteMermaidFenceSource,
} from './markdown/fences.js'
import { parseImageToken, selectedImageToken, rewriteImage, htmlImageAttribute } from './markdown/images.js'
import { safeLinkHref } from './markdown/links.js'
import { detectLineEnding, toEditorText, toFileText, titleForPath } from './markdown/text.js'
import { applyCommand, appendBlock } from './markdown/commands.js'

const BLANK_DOCUMENT = ''

const state = {
  mode: 'wysiwyg', // 'wysiwyg' | 'raw' | 'split'
  path: '',
  dirty: false,
  savedText: '',
  docs: [],
  activeDocId: '',
  fileFilter: '',
  outlineTab: 'outline',
  leftPanelHidden: false,
  rightPanelHidden: false,
  editorContext: {},
  rawOptions: {
    softWrap: true,
    lineNumbers: true,
    hideMarkdownMarkers: false,
  },
  settings: {
    theme: 'light',
    defaultMode: 'wysiwyg',
    showFormattedMarkers: false,
    editorWidth: 72,
    documentFont: 'Public Sans',
    documentFontSize: 15.5,
    documentZoom: 1,
    codeFont: 'JetBrains Mono',
    codeLigatures: true,
    formatOnSave: false,
  },
  systemFonts: [],
  recents: [],
}

const fallbackFonts = ['Public Sans', 'Georgia', 'Menlo', 'Monaco', 'SF Mono', 'JetBrains Mono']

const templates = {
  readme: `# Project Name

Briefly describe what this project does and who it is for.

## Installation

\`\`\`sh
install command
\`\`\`

## Usage

Describe the main workflow.

## API

Document public commands, options, or endpoints.

## License

Add license details.
`,
  changelog: `# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Added

- New item

### Changed

- Updated item

### Fixed

- Fixed item
`,
  'design-doc': `# Design Doc

## Context

Describe the problem and relevant constraints.

## Options

### Option 1

Describe the approach.

### Option 2

Describe the alternative.

## Decision

State the chosen approach and why.

## Consequences

List follow-up work and tradeoffs.
`,
}

const els = {
  wysiwyg: document.getElementById('wysiwyg'),
  raw: document.getElementById('raw'),
  split: document.getElementById('split'),
  splitSource: document.getElementById('split-source'),
  splitSourceHighlight: document.getElementById('split-source-highlight'),
  splitPreview: document.getElementById('split-preview'),
  insertPopoverRoot: document.getElementById('insert-popover-root'),
  fileMenuRoot: document.getElementById('file-menu-root'),
  btnFileMenu: document.getElementById('btn-file-menu'),
  codeAssistantRoot: document.getElementById('code-assistant-root'),
  diagramAssistantRoot: document.getElementById('diagram-assistant-root'),
  settingsRoot: document.getElementById('settings-root'),
  helpRoot: document.getElementById('help-root'),
  exportRoot: document.getElementById('export-root'),
  printRoot: document.getElementById('print-root'),
  workspace: document.getElementById('workspace'),
  fileRail: document.getElementById('file-rail'),
  fileList: document.getElementById('file-list'),
  blockStyle: document.getElementById('block-style'),
  btnToggle: document.getElementById('btn-toggle-mode'),
  btnModeFormatted: document.getElementById('btn-mode-formatted'),
  btnModeRaw: document.getElementById('btn-mode-raw'),
  btnSplit: document.getElementById('btn-split'),
  btnInsertMenu: document.getElementById('btn-insert-menu'),
  btnSettings: document.getElementById('btn-settings'),
  outlinePanel: document.getElementById('outline-panel'),
  btnCloseTab: document.getElementById('btn-close-tab'),
  emptyStart: document.getElementById('empty-start'),
  emptyPaste: document.getElementById('empty-paste'),
  recentFiles: document.getElementById('recent-files'),
  statusMode: document.getElementById('status-mode'),
  statusSync: document.getElementById('status-sync'),
  statusMessage: document.getElementById('status-message'),
  statWords: document.getElementById('stat-words'),
  statReadTime: document.getElementById('stat-read-time'),
  fileSearchInput: document.getElementById('file-search-input'),
  outlineList: document.getElementById('outline-list'),
  ribbonTabs: document.querySelectorAll('[data-ribbon-tab]'),
  zoomControl: document.getElementById('document-zoom'),
  zoomLevel: document.querySelector('[data-zoom-level]'),
  ribbonPanels: document.querySelectorAll('[data-ribbon-panel]'),
  outlineTabs: document.querySelectorAll('[data-outline-tab]'),
}

const wysiwyg = new WysiwygEditor()
const raw = new RawEditor()

let nextDocID = 1
let pushTimer = null
let lastPushedDirty = null
let settingsDraft = null
let settingsTab = 'editor'
let selectedCodeLanguage = 'javascript'
let editingCodeFenceIndex = null
let selectedDiagramType = 'flowchart'
let editingDiagramFenceIndex = null
let syncingSplitScroll = false

const codeLanguages = [
  ['javascript', 'JavaScript'],
  ['typescript', 'TypeScript'],
  ['python', 'Python'],
  ['go', 'Go'],
  ['rust', 'Rust'],
  ['java', 'Java'],
  ['csharp', 'C#'],
  ['cpp', 'C++'],
  ['bash', 'Shell'],
  ['json', 'JSON'],
  ['yaml', 'YAML'],
  ['html', 'HTML'],
  ['css', 'CSS'],
  ['sql', 'SQL'],
  ['markdown', 'Markdown'],
  ['text', 'Plain text'],
]

const codeStarters = {
  javascript: 'const value = "example"',
  typescript: 'const value: string = "example"',
  python: 'value = "example"',
  go: 'value := "example"',
  rust: 'let value = "example";',
  java: 'String value = "example";',
  csharp: 'var value = "example";',
  cpp: 'auto value = "example";',
  bash: 'echo "example"',
  json: '{\n  "value": "example"\n}',
  yaml: 'value: example',
  html: '<div>example</div>',
  css: '.example {\n  color: currentColor;\n}',
  sql: 'select * from documents;',
  markdown: '# Example',
  text: 'plain text',
}

const diagramTemplates = {
  flowchart: {
    label: 'Flowchart',
    description: 'Process steps, decisions, and directional flow.',
    fields: [
      ['start', 'Start step'],
      ['decision', 'Decision'],
      ['yes', 'Yes outcome'],
      ['no', 'No outcome'],
    ],
    body: ({ start, decision, yes, no }) => `graph TD\n  A[${start}] --> B{${decision}}\n  B -->|Yes| C[${yes}]\n  B -->|No| D[${no}]`,
  },
  sequence: {
    label: 'Sequence',
    description: 'Messages exchanged between people or systems.',
    fields: [
      ['participantA', 'First participant'],
      ['participantB', 'Second participant'],
      ['message', 'Message'],
      ['response', 'Response'],
    ],
    body: ({ participantA, participantB, message, response }) => `sequenceDiagram\n  participant ${participantA}\n  participant ${participantB}\n  ${participantA}->>${participantB}: ${message}\n  ${participantB}-->>${participantA}: ${response}`,
  },
  class: {
    label: 'Class',
    description: 'Types, properties, methods, and relationships.',
    fields: [
      ['className', 'Class name'],
      ['property', 'Property'],
      ['method', 'Method'],
    ],
    body: ({ className, property, method }) => `classDiagram\n  class ${className}\n  ${className} : +String ${property}\n  ${className} : +${method}()`,
  },
  state: {
    label: 'State',
    description: 'States and transitions in a workflow.',
    fields: [
      ['first', 'First state'],
      ['second', 'Second state'],
      ['third', 'Third state'],
    ],
    body: ({ first, second, third }) => `stateDiagram-v2\n  [*] --> ${first}\n  ${first} --> ${second}\n  ${second} --> ${third}`,
  },
}

const diagramDrafts = {
  flowchart: { start: 'Start', decision: 'Decision', yes: 'Finish', no: 'Revise' },
  sequence: { participantA: 'User', participantB: 'App', message: 'Request', response: 'Response' },
  class: { className: 'Document', property: 'title', method: 'save' },
  state: { first: 'Draft', second: 'Review', third: 'Published' },
}

function createDoc({ path = '', markdown = BLANK_DOCUMENT, savedText = markdown } = {}) {
  const id = `doc-${nextDocID++}`
  return {
    id,
    path,
    title: titleForPath(path),
    markdown,
    savedText,
    dirty: markdown !== savedText,
    started: markdown.length > 0,
  }
}

function activeDoc() {
  return state.docs.find((doc) => doc.id === state.activeDocId)
}


function getMarkdown() {
  if (state.mode === 'split') return els.splitSource.value
  return state.mode === 'raw' ? raw.getMarkdown() : activeDoc()?.markdown ?? wysiwyg.getMarkdown()
}

async function setMarkdown(md, { markSaved = false } = {}) {
  state.editorContext = {}
  startEditing(md.length > 0)
  if (state.mode === 'split') {
    els.splitSource.value = md
    refreshSplitSourceHighlight()
    await refreshSplitPreview(md)
  } else if (state.mode === 'raw') {
    raw.replaceAll(md)
  } else {
    await renderWysiwyg(md)
  }
  const doc = activeDoc()
  doc.markdown = md
  doc.started = md.length > 0
  if (markSaved) doc.savedText = md
  syncActiveState()
  pushDirtyState()
}

function refreshTitle() {
  document.title = activeDoc()?.title ?? 'Untitled.md'
}

function refreshFileRail() {
  const filter = state.fileFilter.trim().toLowerCase()
  els.fileList.replaceChildren(...state.docs.map((doc) => {
    const button = document.createElement('button')
    button.type = 'button'
    button.className = `rail-row${doc.id === state.activeDocId ? ' active' : ''}`
    button.textContent = doc.title + (doc.dirty ? ' •' : '')
    button.title = doc.path || doc.title
    button.hidden = filter !== '' && !button.textContent.toLowerCase().includes(filter)
    const dot = document.createElement('span')
    button.prepend(dot)
    button.addEventListener('click', () => activateDocument(doc.id))
    return button
  }))
}

function refreshRibbonState() {
  els.btnToggle.textContent = state.mode === 'raw' ? 'WYSIWYG Editor' : 'Raw Markdown'
  els.btnModeFormatted.classList.toggle('active', state.mode === 'wysiwyg')
  els.btnModeRaw.classList.toggle('active', state.mode === 'raw')
  els.btnSplit.classList.toggle('active', state.mode === 'split')
  // aria-checked, not aria-pressed: three exclusive choices are radios. A screen
  // reader should say "2 of 3", not announce three separate toggles that happen
  // to sit next to each other.
  els.btnModeFormatted.setAttribute('aria-checked', state.mode === 'wysiwyg' ? 'true' : 'false')
  els.btnModeRaw.setAttribute('aria-checked', state.mode === 'raw' ? 'true' : 'false')
  els.btnSplit.setAttribute('aria-checked', state.mode === 'split' ? 'true' : 'false')
  els.statusMode.textContent = state.mode === 'raw' ? 'RAW' : state.mode === 'split' ? 'SPLIT' : 'FORMATTED'
  document.body.classList.toggle('raw-mode', state.mode === 'raw')
  document.body.classList.toggle('split-mode', state.mode === 'split')
  document.body.classList.toggle('left-panel-hidden', state.leftPanelHidden)
  document.body.classList.toggle('right-panel-hidden', state.rightPanelHidden)
  document.body.classList.toggle('raw-soft-wrap', state.mode === 'raw' && state.rawOptions.softWrap)
  document.body.classList.toggle('raw-line-numbers', state.mode === 'raw' && state.rawOptions.lineNumbers)
  document.body.classList.toggle('source-hide-markers', state.rawOptions.hideMarkdownMarkers)
  document.querySelectorAll('[data-command="theme"]').forEach((el) => {
    el.classList.toggle('active', document.body.classList.contains('dark'))
  })
  document.querySelectorAll('[data-command="focus"]').forEach((el) => {
    el.classList.toggle('active', document.body.classList.contains('focus'))
  })
}

function refreshDocumentStats(doc) {
  const words = doc?.markdown.trim() ? doc.markdown.trim().split(/\s+/).length : 0
  els.statWords.textContent = String(words)
  els.statReadTime.textContent = `${Math.max(0, Math.ceil(words / 220))} min`
  els.statusSync.textContent = doc?.path ? 'Synced · local' : 'Not synced'
}

function refreshRecentFiles() {
  if (!els.recentFiles) return
  if (state.recents.length === 0) {
    els.recentFiles.replaceChildren()
    els.recentFiles.hidden = true
    return
  }
  els.recentFiles.hidden = false
  const caption = document.createElement('div')
  caption.className = 'empty-caption'
  caption.textContent = 'Recent files'
  const list = document.createElement('div')
  list.className = 'recent-file-list'
  for (const recent of state.recents) {
    const row = document.createElement('button')
    row.type = 'button'
    row.dataset.recentFile = recent.path
    row.title = recent.path
    row.innerHTML = `<strong></strong><span></span>`
    row.querySelector('strong').textContent = recent.title || titleForPath(recent.path)
    row.querySelector('span').textContent = recent.path
    row.addEventListener('click', () => openRecentDocument(recent.path))
    list.append(row)
  }
  els.recentFiles.replaceChildren(caption, list)
}

function toggleSidePanel(side) {
  if (side === 'left') state.leftPanelHidden = !state.leftPanelHidden
  if (side === 'right') state.rightPanelHidden = !state.rightPanelHidden
  refreshRibbonState()
}

function refreshOutlinePanel(doc) {
  if (state.mode === 'raw') {
    refreshRawPanel()
    return
  }
  els.outlinePanel.replaceChildren(
    panelRestoreButton('right'),
    sidePanelHeader('Document', 'right'),
    outlineTabsElement(),
    outlineListElement(),
    documentStatsElement()
  )
  els.outlineTabs.forEach((tab) => {
    const active = tab.dataset.outlineTab === state.outlineTab
    tab.classList.toggle('active', active)
    tab.setAttribute('aria-selected', active ? 'true' : 'false')
  })

  const rows = state.outlineTab === 'links'
    ? linkRows(doc?.markdown ?? '')
    : state.outlineTab === 'comments'
      ? [{ text: 'No comments in this document', muted: true }]
      : outlineRows(doc)

  els.outlineList.replaceChildren(...rows.map((row, index) => {
    const el = document.createElement('div')
    el.className = `outline-row${index === 0 && !row.muted ? ' active' : ''}${row.muted ? ' muted' : ''}`
    el.textContent = row.text
    if (row.level) el.style.paddingLeft = `${8 + (row.level - 1) * 12}px`
    return el
  }))
}

function refreshRawPanel() {
  els.outlinePanel.replaceChildren(
    panelRestoreButton('right'),
    sidePanelHeader('Source', 'right'),
    syntaxLegendElement(),
    rawEditorTogglesElement()
  )
}

function sidePanelHeader(label, side) {
  const header = document.createElement('div')
  header.className = 'side-panel-header'
  const text = document.createElement('span')
  text.textContent = label
  header.append(text, panelToggleButton(side, side === 'left' ? '‹' : '›'))
  return header
}

function panelRestoreButton(side) {
  const button = panelToggleButton(side, side === 'left' ? '›' : '‹')
  button.classList.add('panel-restore')
  button.textContent = side === 'left' ? 'Files' : 'Document'
  button.setAttribute('aria-label', side === 'left' ? 'Show file panel' : 'Show outline panel')
  button.title = side === 'left' ? 'Show file panel' : 'Show outline panel'
  return button
}

function panelToggleButton(side, label) {
  const button = document.createElement('button')
  button.className = 'panel-toggle'
  button.type = 'button'
  button.dataset.panelToggle = side
  button.textContent = label
  button.setAttribute('aria-label', side === 'left' ? 'Hide file panel' : 'Hide outline panel')
  button.title = side === 'left' ? 'Hide file panel' : 'Hide outline panel'
  return button
}

function outlineTabsElement() {
  const nav = document.createElement('nav')
  nav.className = 'outline-tabs'
  for (const [tab, label] of [['outline', 'Outline'], ['comments', 'Comments'], ['links', 'Links']]) {
    const button = document.createElement('button')
    button.type = 'button'
    button.dataset.outlineTab = tab
    button.textContent = label
    button.addEventListener('click', () => {
      state.outlineTab = tab
      refreshOutlinePanel(activeDoc())
    })
    nav.append(button)
  }
  els.outlineTabs = nav.querySelectorAll('[data-outline-tab]')
  return nav
}

function outlineListElement() {
  const list = document.createElement('div')
  list.id = 'outline-list'
  els.outlineList = list
  return list
}

function documentStatsElement() {
  const stats = document.createElement('div')
  stats.className = 'document-stats'
  stats.innerHTML = `
    <div class="rail-caption">Document</div>
    <div><span>Words</span><span id="stat-words">0</span></div>
    <div><span>Read time</span><span id="stat-read-time">0 min</span></div>
    <div><span>Lint</span><span class="clean">clean</span></div>
  `
  els.statWords = stats.querySelector('#stat-words')
  els.statReadTime = stats.querySelector('#stat-read-time')
  return stats
}

function syntaxLegendElement() {
  const panel = document.createElement('div')
  panel.className = 'syntax-panel'
  panel.dataset.rawPanel = 'syntax'
  panel.innerHTML = `
    <div class="rail-caption">Syntax Legend</div>
    <div class="syntax-row"><span class="syntax-heading"># ##</span><span>Headings</span></div>
    <div class="syntax-row"><span class="syntax-code">\`\`\`</span><span>Code</span></div>
    <div class="syntax-row"><span class="syntax-link">[ ] ( )</span><span>Links</span></div>
    <div class="syntax-row"><span class="syntax-muted">** _</span><span>Emphasis marks</span></div>
  `
  return panel
}

function rawEditorTogglesElement() {
  const panel = document.createElement('div')
  panel.className = 'raw-options'
  panel.innerHTML = `
    <div class="rail-caption">Editor</div>
    ${rawToggleRow('softWrap', 'Soft wrap', state.rawOptions.softWrap)}
    ${rawToggleRow('lineNumbers', 'Line numbers', state.rawOptions.lineNumbers)}
    ${rawToggleRow('hideMarkdownMarkers', 'Hide markers', state.rawOptions.hideMarkdownMarkers)}
  `
  panel.querySelectorAll('[data-raw-toggle]').forEach((toggle) => {
    toggle.addEventListener('change', () => setRawOption(toggle.dataset.rawToggle, toggle.checked))
  })
  return panel
}

function rawToggleRow(name, label, checked) {
  return `
    <label class="raw-toggle-row">
      <span>${label}</span>
      <input type="checkbox" data-raw-toggle="${name}" ${checked ? 'checked' : ''}>
    </label>
  `
}

function outlineRows(doc) {
  const headingRows = (doc?.markdown ?? '').split('\n')
    .map((line) => line.match(/^(#{1,6})\s+(.+?)\s*#*\s*$/))
    .filter(Boolean)
    .map((match) => ({ level: match[1].length, text: match[2] }))
  if (headingRows.length > 0) return headingRows
  return [{ text: doc?.title?.replace(/\.md$/i, '') || 'Untitled', level: 1 }]
}

function linkRows(md) {
  const rows = []
  const pattern = /\[([^\]]+)\]\(([^)]+)\)/g
  let match = pattern.exec(md)
  while (match) {
    rows.push({ text: `${match[1]} · ${match[2]}` })
    match = pattern.exec(md)
  }
  return rows.length > 0 ? rows : [{ text: 'No links in this document', muted: true }]
}

function syncActiveState() {
  const doc = activeDoc()
  state.path = doc?.path ?? ''
  state.savedText = doc?.savedText ?? ''
  state.dirty = doc ? doc.markdown !== doc.savedText : false
  refreshTitle()
  refreshFileRail()
  refreshRibbonState()
  refreshOutlinePanel(doc)
  refreshDocumentStats(doc)
  refreshRecentFiles()
  document.body.classList.toggle(
    'app-empty',
    Boolean(doc && !doc.started && doc.markdown.trim() === '' && state.mode === 'wysiwyg')
  )
}

function handleRenderedBlockSelection(event) {
  if (state.mode !== 'wysiwyg') return
  // Delegated, because the editor's node view mounts a COPY of the preview
  // element the app hands it, and a listener attached to the original is not
  // on the copy that is clicked.
  if (event.target.closest?.('[data-diagram-edit]')) return handleDiagramEditRequest(event)
  const table = event.target.closest?.('#wysiwyg table')
  const diagram = event.target.closest?.('#wysiwyg .mermaid-render')
  const image = event.target.closest?.('#wysiwyg img')
  if (image) {
    const imageIndex = Array.from(els.wysiwyg.querySelectorAll('img')).indexOf(image)
    if (imageIndex < 0) return
    state.editorContext = { blockType: 'image', imageIndex }
  } else if (table) {
    const tables = Array.from(els.wysiwyg.querySelectorAll('table'))
    const tableIndex = tables.indexOf(table)
    if (tableIndex < 0) return
    state.editorContext = { blockType: 'table', tableIndex }
  } else if (diagram) {
    const diagramFenceIndex = renderedCodeBlockIndex(diagram)
    if (diagramFenceIndex < 0) return
    state.editorContext = { blockType: 'diagram', diagramFenceIndex }
  } else {
    return
  }
  refreshRenderedBlockSelection()
}

// The diagram's own Edit control, announced from inside the rendered preview.
//
// The preview element belongs to the app (editor.js hands it to the node view),
// so the button lives on the block it edits rather than in a bar floating over
// the document. The fence index is resolved from where the click came from, so
// it always targets THAT diagram — the floating bar resolved the first matching
// fence in the document, which edited the wrong block whenever there were two.
function handleDiagramEditRequest(event) {
  const diagramFenceIndex = renderedCodeBlockIndex(event.target)
  if (diagramFenceIndex < 0) return
  state.editorContext = { blockType: 'diagram', diagramFenceIndex }
  openDiagramAssistant({ editFenceIndex: diagramFenceIndex })
}

// Which fenced block, counting from the top of the document, a node inside the
// formatted surface belongs to.
//
// This is the index every fence rewrite in this app takes — `rewriteMermaidFenceSource`
// and `rewriteCodeFenceLanguage` both count ALL fences, mermaid or not — so it
// has to be the position among all code blocks, not among the diagrams.
//
// The editor renders one `.milkdown-code-block` per fenced block in document
// order, and it keeps that element whether the block is showing its diagram or
// its source. Counting them is therefore exact where the previous approach was
// not: the app used to stamp the index onto the diagram element it drew itself,
// which stopped being possible when the editor took ownership of the block, and
// counting only the rendered diagrams would have shifted every index the moment
// one block was toggled to source.
function renderedCodeBlockIndex(node) {
  const block = node.closest('.milkdown-code-block')
  if (!block) return -1
  return Array.from(els.wysiwyg.querySelectorAll('.milkdown-code-block')).indexOf(block)
}

function refreshRenderedBlockSelection() {
  els.wysiwyg.querySelectorAll('table').forEach((table, index) => {
    table.classList.toggle('selected-block', state.editorContext.blockType === 'table' && state.editorContext.tableIndex === index)
  })
  els.wysiwyg.querySelectorAll('.mermaid-render').forEach((diagram) => {
    diagram.classList.toggle('selected-block', state.editorContext.blockType === 'diagram' && renderedCodeBlockIndex(diagram) === state.editorContext.diagramFenceIndex)
  })
  els.wysiwyg.querySelectorAll('img').forEach((image, index) => {
    image.classList.toggle('selected-block', state.editorContext.blockType === 'image' && state.editorContext.imageIndex === index)
  })
}

// Marks the active document as started, so the placeholder gives way.
//
// This took a `started` parameter that was never passed anything but true, and
// its body then read `started || doc.markdown.length > 0` — with `started`
// always true, the right-hand side was unreachable. Dead configurability with a
// dead branch behind it, reading as a choice the caller gets to make.
function startEditing() {
  const doc = activeDoc()
  if (doc) doc.started = true
}

// Document zoom, in the sense Word means it: everything in the pane gets
// closer, proportions preserved.
//
// Implemented with CSS `zoom` rather than `transform: scale`, because zoom takes
// part in LAYOUT. Measured: at 1.5 the host went 720px -> 1080px wide and the
// pane's scrollHeight grew 759 -> 848, so the content scrolls to reach. A
// transform paints at a different size while the parent still lays out the
// original, so a zoomed-in document would simply overflow with nothing to
// scroll to. It also keeps text crisp, being a real layout at the new size
// rather than a rasterised one.
//
// The control lives in the pane but OUTSIDE #editor-host, or it would zoom
// itself and shrink as you zoomed out.
const ZOOM_MIN = 0.5
const ZOOM_MAX = 2
const ZOOM_STEP = 0.1

function setDocumentZoom(zoom) {
  const clamped = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, Math.round(zoom * 100) / 100))
  state.settings.documentZoom = clamped
  document.documentElement.style.setProperty('--doc-zoom', String(clamped))
  refreshZoomControl()
  persistSettings()
}

function refreshZoomControl() {
  if (!els.zoomLevel) return
  const zoom = state.settings.documentZoom ?? 1
  els.zoomLevel.textContent = `${Math.round(zoom * 100)}%`
  // A disabled control that still looks pressable is the honest-controls rule:
  // at the ends of the range these do nothing, and should say so.
  els.zoomControl?.querySelector('[data-zoom="out"]')?.toggleAttribute('disabled', zoom <= ZOOM_MIN)
  els.zoomControl?.querySelector('[data-zoom="in"]')?.toggleAttribute('disabled', zoom >= ZOOM_MAX)
}

function wireZoomControl() {
  els.zoomControl?.addEventListener('click', (event) => {
    const action = event.target.closest('[data-zoom]')?.dataset.zoom
    if (!action) return
    const zoom = state.settings.documentZoom ?? 1
    if (action === 'in') setDocumentZoom(zoom + ZOOM_STEP)
    else if (action === 'out') setDocumentZoom(zoom - ZOOM_STEP)
    else setDocumentZoom(1)
  })
}

function activateRibbonTab(name) {
  els.ribbonTabs.forEach((tab) => {
    const active = tab.dataset.ribbonTab === name
    tab.classList.toggle('active', active)
    tab.setAttribute('aria-selected', active ? 'true' : 'false')
  })
  els.ribbonPanels.forEach((panel) => {
    panel.hidden = panel.dataset.ribbonPanel !== name
  })
}

function suppressBrowserDefaults() {
  document.addEventListener('contextmenu', (event) => {
    event.preventDefault()
  })
  document.addEventListener('dragover', (event) => {
    event.preventDefault()
  })
  document.addEventListener('drop', (event) => {
    event.preventDefault()
  })
  document.addEventListener('dragstart', (event) => {
    if (!event.target.closest('#wysiwyg, #raw')) event.preventDefault()
  })
  // The host delivers dropped files with real filesystem paths through a host
  // event; the DOM drop event above carries no usable path.
  globalThis.drmd?.events?.on?.('files:dropped', (paths) => handleDroppedFiles(paths))
}

async function persistCurrentEditorText() {
  const doc = activeDoc()
  if (!doc) return
  doc.markdown = getMarkdown()
  doc.dirty = doc.markdown !== doc.savedText
  syncActiveState()
}

async function activateDocument(id) {
  if (id === state.activeDocId) return
  cancelPendingPush()
  await persistCurrentEditorText()
  const next = state.docs.find((doc) => doc.id === id)
  if (!next) return
  state.editorContext = {}
  state.activeDocId = id
  await mountMarkdown(next.markdown)
  syncActiveState()
  pushDirtyState()
}

async function mountMarkdown(md) {
  if (state.mode === 'split') {
    els.splitSource.value = md
    refreshSplitSourceHighlight()
    await refreshSplitPreview(md)
  } else if (state.mode === 'raw') {
    raw.replaceAll(md)
  } else {
    await renderWysiwyg(md)
    refreshRenderedBlockSelection()
  }
}

// Renders are serialized and superseded renders abandon their work.
//
// The three passes below each await, so a render could be suspended while a
// newer one started — and the older pass then resumed and stamped its state
// over the newer DOM. Reproduced deterministically: two renders started back to
// back left a one-block document with two code shells, one carrying the
// language from the superseded render. In normal use it showed up as a stale
// syntax highlight after changing a code block's language, which never
// corrected itself because nothing rendered again.
//
// The generation check is what makes this correct, not the chaining. Chaining
// alone would still run every queued render to completion; the check lets an
// obsolete one stop at its next suspension point, and lets the newest win.
let renderGeneration = 0
let renderQueue = Promise.resolve()

// Every WYSIWYG render runs the same two passes; keeping them together stops
// a new call site from silently skipping one (imported images did exactly that).
//
// Code blocks used to be a third pass here, rewriting the editor's own nodes to
// add highlighting and chrome. They are not any more: the editor's code-mirror
// node view renders, highlights and edits them itself, and draws mermaid
// diagrams through the preview hook in editor.js. The pass had to go, not just
// become redundant — it replaced nodes the node view owns, so whichever ran
// second won, and the block was left either inert or undecorated (#77).
function renderWysiwyg(md) {
  const generation = ++renderGeneration
  const superseded = () => generation !== renderGeneration

  renderQueue = renderQueue.then(async () => {
    // Already obsolete before this render's turn came up: skip it entirely
    // rather than rebuild the editor twice for a result nobody will see.
    if (superseded()) return
    await wysiwyg.setMarkdown(els.wysiwyg, md)
    if (superseded()) return
    await resolveImageAssets(els.wysiwyg)
  })

  // Serializing through one promise makes a rejection contagious: every later
  // `.then` would skip its callback, so a single failed render would stop the
  // surface updating for the rest of the session — silently, with the app
  // appearing to ignore every edit. Absorb it here to keep the queue alive, and
  // record it rather than swallow it, so the failure is recoverable after the
  // fact instead of invisible.
  renderQueue = renderQueue.catch((error) => {
    console.error('render failed', error)
    bridge.recordEvent('render.failed', { error: String(error?.message ?? error) })
  })
  return renderQueue
}

// Markdown resolves relative image paths against the document on disk, but the
// webview resolves them against the asset-server origin. Every rendering
// surface therefore routes local images through the bridge, which inlines the
// bytes; that also makes print/export artifacts self-contained.
async function resolveImageAssets(root) {
  if (!root) return
  const documentPath = activeDoc()?.path ?? ''
  for (const img of Array.from(root.querySelectorAll('img[src]'))) {
    const source = img.getAttribute('src') ?? ''
    if (!source || /^(?:https?:|data:|file:|blob:)/i.test(source)) continue
    img.dataset.assetPath = source
    let loaded = null
    try {
      loaded = await bridge.loadImageAsset(documentPath, source)
    } catch {
      loaded = null
    }
    if (loaded?.exists && loaded.dataURI) {
      img.src = loaded.dataURI
      delete img.dataset.missingAsset
    } else {
      // Keep the markdown path visible so a lost asset reads as lost, not blank.
      img.dataset.missingAsset = 'true'
      img.removeAttribute('src')
      if (!img.alt) img.alt = source
    }
  }
}

async function toggleMode() {
  return setMode(state.mode === 'raw' ? 'wysiwyg' : 'raw')
}

async function toggleSplit() {
  return setMode(state.mode === 'split' ? 'wysiwyg' : 'split')
}

async function setMode(mode) {
  if (mode === state.mode) return
  await persistCurrentEditorText()
  startEditing()
  const md = activeDoc().markdown
  if (state.mode === 'raw') raw.close()
  els.wysiwyg.hidden = true
  els.raw.hidden = true
  els.split.hidden = true

  if (mode === 'raw') {
    els.raw.hidden = false
    raw.open(els.raw, md, onEdited, state.rawOptions)
  } else if (mode === 'split') {
    els.split.hidden = false
    els.splitSource.value = md
    refreshSplitSourceHighlight()
    await refreshSplitPreview(md)
  } else {
    els.wysiwyg.hidden = false
    await renderWysiwyg(md)
  }
  state.mode = mode
  syncActiveState()
}

// --- files ---

async function newDocument() {
  cancelPendingPush()
  await persistCurrentEditorText()
  const doc = createDoc({ markdown: '', savedText: '' })
  state.docs.push(doc)
  state.activeDocId = doc.id
  await setMode(state.settings.defaultMode)
  await mountMarkdown('')
  syncActiveState()
  pushDirtyState()
}





async function openDocument() {
  if (state.dirty) {
    const proceed = await bridge.resolveUnsavedChanges()
    if (!proceed) return
  }
  await persistCurrentEditorText()
  cancelPendingPush()
  const res = await bridge.openDocument()
  if (!res || (!res.path && !res.content)) return
  const doc = activeDoc()
  doc.path = res.path
  doc.title = titleForPath(res.path)
  doc.lineEnding = detectLineEnding(res.content)
  doc.markdown = toEditorText(res.content)
  doc.savedText = doc.markdown
  doc.started = true
  await mountMarkdown(doc.markdown)
  syncActiveState()
  bridge.setDirty(false)
  lastPushedDirty = false
  await refreshNativePreferences()
}

async function openRecentDocument(path) {
  if (state.dirty) {
    const proceed = await bridge.resolveUnsavedChanges()
    if (!proceed) return
  }
  await persistCurrentEditorText()
  cancelPendingPush()
  const res = await bridge.openRecentDocument(path)
  if (!res || (!res.path && !res.content)) return
  const doc = activeDoc()
  doc.path = res.path
  doc.title = titleForPath(res.path)
  doc.lineEnding = detectLineEnding(res.content)
  doc.markdown = toEditorText(res.content)
  doc.savedText = doc.markdown
  doc.started = true
  await mountMarkdown(doc.markdown)
  syncActiveState()
  bridge.setDirty(false)
  lastPushedDirty = false
  await refreshNativePreferences()
}

async function save() {
  await persistCurrentEditorText()
  const doc = activeDoc()
  await formatDocumentIfRequested(doc)
  if (!doc.path) return saveAs()
  await bridge.saveDocument(doc.path, toFileText(doc.markdown, doc.lineEnding))
  cancelPendingPush()
  doc.savedText = doc.markdown
  syncActiveState()
  bridge.setDirty(false)
  lastPushedDirty = false
  await refreshNativePreferences()
}

async function saveAs() {
  await persistCurrentEditorText()
  const doc = activeDoc()
  await formatDocumentIfRequested(doc)
  const path = await bridge.saveDocumentAs(toFileText(doc.markdown, doc.lineEnding))
  if (!path) return
  cancelPendingPush()
  doc.path = path
  doc.title = titleForPath(path)
  doc.savedText = doc.markdown
  syncActiveState()
  bridge.setDirty(false)
  lastPushedDirty = false
  await refreshNativePreferences()
}

async function formatDocumentIfRequested(doc) {
  if (!state.settings.formatOnSave) return
  const formatted = formatMarkdownForSave(doc.markdown)
  if (formatted === doc.markdown) return
  doc.markdown = formatted
  await mountMarkdown(formatted)
}

function formatMarkdownForSave(md) {
  return `${md.split('\n').map((line) => line.trimEnd()).join('\n').trimEnd()}\n`
}

async function closeActiveTab() {
  // Cancel the debounced UpdateContent before the tab goes away. It carries
  // THIS tab's text; surviving the close means it lands after the next tab is
  // active and Go writes the closed tab's content into the surviving tab's
  // file. Every other document transition (activate, new, open, save, saveAs)
  // already cancels — this was the one that did not, and it reproduced the
  // cross-document overwrite the tab-identity fix was meant to end.
  cancelPendingPush()
  if (state.dirty) {
    const proceed = await bridge.resolveUnsavedChanges()
    if (!proceed) return
  }
  await persistCurrentEditorText()
  const index = state.docs.findIndex((doc) => doc.id === state.activeDocId)
  state.docs.splice(index, 1)
  if (state.docs.length === 0) state.docs.push(createDoc())
  state.activeDocId = state.docs[Math.max(0, index - 1)].id
  await mountMarkdown(activeDoc().markdown)
  syncActiveState()
  pushDirtyState()
}

// --- ribbon commands ---

async function runCommand(command) {
  if (command === 'new') return newDocument()
  if (command === 'close-tab') return closeActiveTab()
  if (command === 'theme') return toggleTheme()
  if (command === 'focus') return toggleFocus()
  if (command === 'image') return insertImage()
  if (command === 'image-replace') return replaceSelectedImage()
  if (command === 'image-reveal') return revealSelectedImage()

  const editorContext = currentEditorContext()
  await persistCurrentEditorText()
  startEditing()
  const doc = activeDoc()
  doc.markdown = applyCommand(command, doc.markdown, editorContext)
  state.editorContext = {}
  await mountMarkdown(doc.markdown)
  markEdited(doc.markdown)
}

// Extensions the asset importer accepts; the same set the native picker filters
// on. Anything else dropped on the window is left alone.
const IMPORTABLE_IMAGE_EXTENSIONS = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.svg']

// handleDroppedFiles is driven by the host's file-drop event, which
// supplies real filesystem paths (a browser File object has none).
async function handleDroppedFiles(paths) {
  const images = (paths ?? []).filter((path) =>
    IMPORTABLE_IMAGE_EXTENSIONS.some((extension) => path.toLowerCase().endsWith(extension)))
  if (images.length === 0) return
  await persistCurrentEditorText()
  startEditing()
  const doc = activeDoc()
  if (!doc) return
  for (const path of images) {
    let result = null
    try {
      result = await bridge.importDroppedImage(doc.path, path)
    } catch (error) {
      // Rejections (unsaved document, unreadable source) are already reported
      // natively; stop rather than half-importing the rest of the drop.
      console.warn('bridge: dropped image import rejected', error)
      bridge.recordEvent('image.import-failed', { source: 'drop', error: String(error?.message ?? error) })
      return
    }
    if (!result?.markdown) continue
    doc.markdown = appendBlock(doc.markdown, result.markdown)
  }
  await mountMarkdown(doc.markdown)
  markEdited(doc.markdown)
}

async function insertImage() {
  await persistCurrentEditorText()
  startEditing()
  const doc = activeDoc()
  let result
  try {
    result = await bridge.importImage(doc.path)
  } catch (error) {
    // The import already reported itself through the native error dialog
    // (unsaved document, unreadable source, failed copy). Leave the document
    // untouched rather than inserting a non-portable placeholder.
    console.warn('bridge: image import rejected', error)
    bridge.recordEvent('image.import-failed', { source: 'ribbon', error: String(error?.message ?? error) })
    return
  }
  if (!result?.markdown) return
  doc.markdown = appendBlock(doc.markdown, result.markdown)
  await mountMarkdown(doc.markdown)
  markEdited(doc.markdown)
}

// applyImageEdit rewrites the selected image and remounts, sharing the single
// path used by every image-local operation.
async function applyImageEdit(transform) {
  await persistCurrentEditorText()
  const doc = activeDoc()
  if (!doc) return
  const next = rewriteImage(doc.markdown, state.editorContext.imageIndex, transform)
  if (next === doc.markdown) return
  doc.markdown = next
  await mountMarkdown(doc.markdown)
  markEdited(doc.markdown)
}

async function setImageAltText(alt) {
  await applyImageEdit((image) => ({ ...image, alt }))
}

async function setImageWidth(width) {
  await applyImageEdit((image) => ({ ...image, width }))
}

async function deleteSelectedImage() {
  await applyImageEdit(() => null)
}

async function replaceSelectedImage() {
  await persistCurrentEditorText()
  const doc = activeDoc()
  if (!doc) return
  let result = null
  try {
    result = await bridge.importImage(doc.path)
  } catch (error) {
    console.warn('bridge: image replace rejected', error)
    bridge.recordEvent('image.replace-failed', { error: String(error?.message ?? error) })
    return
  }
  if (!result?.markdownPath) return
  await applyImageEdit((image) => ({ ...image, path: result.markdownPath }))
}

// How long a transient status message stays up. Long enough to read a short
// sentence, short enough that it is gone before it becomes furniture.
const STATUS_MESSAGE_MS = 3200
let statusMessageTimer = null

// Say something to the user, briefly.
//
// Added because a menu command that silently does nothing when it does not
// apply is the shape of #75 — a control that renders and has no effect — and
// this application had nowhere at all to say why. It is also the first time an
// image reveal FAILURE becomes visible: that path used to console.warn, which
// in a production build with no devtools is the same as saying nothing.
function flashStatus(text) {
  if (!els.statusMessage) return
  els.statusMessage.textContent = text
  els.statusMessage.classList.add('showing')
  clearTimeout(statusMessageTimer)
  statusMessageTimer = setTimeout(() => {
    els.statusMessage.classList.remove('showing')
  }, STATUS_MESSAGE_MS)
}

async function revealSelectedImage() {
  const doc = activeDoc()
  const target = selectedImageToken(doc?.markdown ?? '', state.editorContext.imageIndex)
  if (!target) {
    // Reachable from the View menu, which cannot know whether an image is
    // selected. Saying so is the difference between a command that does not
    // apply and a command that is broken.
    flashStatus('Select an image first, then Reveal in Finder.')
    return
  }
  try {
    await bridge.revealImageAsset(doc.path ?? '', parseImageToken(target.text).path)
  } catch (error) {
    flashStatus('That image could not be revealed.')
    console.warn('bridge: image reveal rejected', error)
    bridge.recordEvent('image.reveal-failed', { error: String(error?.message ?? error) })
  }
}


function currentEditorContext() {
  const selection = window.getSelection()
  if (!selection || selection.rangeCount === 0) return state.editorContext
  const anchor = selection.anchorNode
  const focus = selection.focusNode
  const anchorInEditor = anchor && (els.wysiwyg.contains(anchor) || els.raw.contains(anchor))
  const focusInEditor = focus && (els.wysiwyg.contains(focus) || els.raw.contains(focus))
  if (!anchorInEditor || !focusInEditor) return state.editorContext
  state.editorContext = {
    selectionText: selection.isCollapsed ? '' : selection.toString(),
    blockText: nearestEditorBlockText(anchor),
  }
  return state.editorContext
}

function nearestEditorBlockText(node) {
  const element = node.nodeType === Node.TEXT_NODE ? node.parentElement : node
  const block = element?.closest?.('h1,h2,h3,h4,h5,h6,p,li,blockquote,pre,code')
  return block?.textContent ?? element?.textContent ?? ''
}









function applyBlockStyle(style) {
  runCommand(style)
}























function toggleTheme() {
  state.settings.theme = document.body.classList.contains('dark') ? 'light' : 'dark'
  applyRuntimeSettings()
  refreshRibbonState()
}

function toggleFocus() {
  document.body.classList.toggle('focus')
  refreshRibbonState()
}

async function setRawOption(name, value) {
  if (!(name in state.rawOptions)) return
  state.rawOptions[name] = value
  refreshRawOptionState()
}

function applyRuntimeSettings() {
  document.body.classList.toggle('dark', state.settings.theme === 'dark')
  document.documentElement.style.setProperty('--document-font', fontStack(state.settings.documentFont, 'var(--ui)'))
  document.documentElement.style.setProperty('--code-font', fontStack(state.settings.codeFont, 'ui-monospace, SFMono-Regular, Menlo, Monaco, monospace'))
  document.documentElement.style.setProperty('--mono', fontStack(state.settings.codeFont, 'ui-monospace, SFMono-Regular, Menlo, Monaco, monospace'))
  document.documentElement.style.setProperty('--document-font-size', `${state.settings.documentFontSize}px`)
  document.documentElement.style.setProperty('--editor-width', `${state.settings.editorWidth}ch`)
  document.documentElement.style.setProperty('--doc-zoom', String(state.settings.documentZoom))
  refreshZoomControl()
  document.documentElement.style.setProperty('--code-ligatures', state.settings.codeLigatures ? 'common-ligatures' : 'none')
  document.documentElement.style.setProperty('--code-font-features', state.settings.codeLigatures ? 'normal' : '"liga" 0, "calt" 0')
  document.body.classList.toggle('show-formatted-markers', state.settings.showFormattedMarkers)
}

async function loadNativePreferences() {
  let prefs = null
  try {
    prefs = await bridge.loadPreferences()
  } catch (error) {
    // Preferences are an enhancement; the editor is the product. A store the
    // app cannot read must never stop it starting (issue #17).
    console.warn('preferences load failed; starting with defaults', error)
    bridge.recordEvent('preferences.load-failed', { error: String(error?.message ?? error) })
    return
  }
  mergeNativePreferences(prefs)
}

async function refreshNativePreferences() {
  const prefs = await bridge.loadPreferences()
  if (prefs?.recents) {
    state.recents = normalizeRecents(prefs.recents)
    refreshRecentFiles()
  }
}

function mergeNativePreferences(prefs) {
  if (!prefs || typeof prefs !== 'object') return
  if (prefs.settings && typeof prefs.settings === 'object') {
    state.settings = { ...state.settings, ...prefs.settings }
  }
  if (prefs.rawOptions && typeof prefs.rawOptions === 'object') {
    state.rawOptions = { ...state.rawOptions, ...prefs.rawOptions }
  }
  state.recents = normalizeRecents(prefs.recents)
}

function normalizeRecents(recents) {
  if (!Array.isArray(recents)) return []
  return recents
    .filter((recent) => recent && typeof recent.path === 'string' && recent.path.trim() !== '')
    .map((recent) => ({
      path: recent.path,
      title: recent.title || titleForPath(recent.path),
      lastOpenedAt: recent.lastOpenedAt || '',
    }))
}

function preferencesEnvelope() {
  return {
    settings: { ...state.settings },
    rawOptions: { ...state.rawOptions },
    recents: state.recents.map((recent) => ({ ...recent })),
  }
}

function refreshRawOptionState() {
  refreshRibbonState()
  if (state.mode === 'raw') {
    const md = raw.getMarkdown()
    raw.close()
    raw.open(els.raw, md, onEdited, state.rawOptions)
    refreshRawPanel()
  }
  if (state.mode === 'split') {
    refreshSplitSourceHighlight()
  }
}

async function openSettings() {
  await loadSystemFonts()
  settingsDraft = {
    settings: { ...state.settings },
    rawOptions: { ...state.rawOptions },
  }
  settingsTab = 'editor'
  renderSettingsModal()
}

async function loadSystemFonts() {
  if (state.systemFonts.length > 0) return
  const fonts = await bridge.listFontFamilies()
  state.systemFonts = normalizeFontList(Array.isArray(fonts) ? fonts : fallbackFonts)
}

function normalizeFontList(fonts) {
  return Array.from(new Set([...fonts, ...fallbackFonts].map((font) => String(font).trim()).filter(Boolean))).sort((a, b) =>
    a.localeCompare(b, undefined, { sensitivity: 'base' })
  )
}

function fontStack(font, fallback) {
  return `"${String(font).replace(/"/g, '\\"')}", ${fallback}`
}

function closeSettings() {
  settingsDraft = null
  els.settingsRoot.replaceChildren()
}

function focusFirstControl(root) {
  root.querySelector('button:not([disabled]), select:not([disabled]), input:not([disabled]), textarea:not([disabled])')?.focus()
}

// Writes the current preferences through the bridge.
//
// Extracted from saveSettings so the zoom control can persist without going
// through the settings modal's draft-and-close flow. A failure is contained but
// recorded: a production build has no devtools, so console alone reaches nobody.
function persistSettings() {
  const pending = bridge.savePreferences(preferencesEnvelope())
  if (pending?.catch) {
    pending.catch((err) => {
      console.warn('preferences save failed', err)
      bridge.recordEvent('preferences.save-failed', { error: String(err?.message ?? err) })
    })
  }
}

function saveSettings() {
  if (!settingsDraft) return
  state.settings = { ...settingsDraft.settings }
  state.rawOptions = { ...settingsDraft.rawOptions }
  applyRuntimeSettings()
  refreshRawOptionState()
  persistSettings()
  closeSettings()
}

function renderSettingsModal() {
  const modal = document.createElement('div')
  modal.className = 'settings-scrim'
  modal.dataset.settingsModal = 'true'
  modal.innerHTML = `
    <section class="settings-dialog" role="dialog" aria-modal="true" aria-labelledby="settings-title">
      <nav class="settings-nav" aria-label="Settings sections">
        <div class="settings-nav-title">Preferences</div>
        ${settingsNavButton('editor', 'Editor')}
        ${settingsNavButton('appearance', 'Appearance')}
        ${settingsNavButton('markdown', 'Markdown flavour', true)}
        ${settingsNavButton('shortcuts', 'Shortcuts')}
        ${settingsNavButton('sync', 'Sync & Git', true)}
        ${settingsNavButton('extensions', 'Extensions', true)}
      </nav>
      <div class="settings-pane">
        <div class="settings-content"></div>
        <footer class="settings-footer">
          <button type="button" data-settings-action="cancel">Cancel</button>
          <button class="primary" type="button" data-settings-action="save">Save changes</button>
        </footer>
      </div>
    </section>
  `
  modal.querySelectorAll('[data-settings-nav]').forEach((button) => {
    button.addEventListener('click', () => {
      settingsTab = button.dataset.settingsNav
      renderSettingsContent(modal)
    })
  })
  modal.querySelector('[data-settings-action="cancel"]').addEventListener('click', closeSettings)
  modal.querySelector('[data-settings-action="save"]').addEventListener('click', saveSettings)
  els.settingsRoot.replaceChildren(modal)
  renderSettingsContent(modal)
  focusFirstControl(modal)
}

function settingsNavButton(tab, label, disabled = false) {
  return `<button type="button" data-settings-nav="${tab}" ${disabled ? 'disabled aria-disabled="true"' : ''}>${label}</button>`
}

function renderSettingsContent(modal = els.settingsRoot.querySelector('[data-settings-modal]')) {
  if (!modal || !settingsDraft) return
  modal.querySelectorAll('[data-settings-nav]').forEach((button) => {
    button.classList.toggle('active', button.dataset.settingsNav === settingsTab)
  })
  const content = modal.querySelector('.settings-content')
  if (settingsTab === 'appearance') renderAppearanceSettings(content)
  else if (settingsTab === 'shortcuts') renderShortcutSettings(content)
  else if (['markdown', 'sync', 'extensions'].includes(settingsTab)) renderUnavailableSettings(content, settingsTitleForTab(settingsTab))
  else renderEditorSettings(content)
}

function renderEditorSettings(content) {
  content.innerHTML = settingsHeader('Editor', 'How the source editor behaves while you write.') + `
    <div class="settings-rows">
      <label class="settings-row">
        <div><strong>Default mode</strong><span>View new documents open in.</span></div>
        <select data-settings-field="defaultMode">
          <option value="wysiwyg" ${settingsDraft.settings.defaultMode === 'wysiwyg' ? 'selected' : ''}>Formatted</option>
          <option value="raw" ${settingsDraft.settings.defaultMode === 'raw' ? 'selected' : ''}>Raw</option>
          <option value="split" ${settingsDraft.settings.defaultMode === 'split' ? 'selected' : ''}>Split</option>
        </select>
      </label>
      ${settingsToggleRow('showFormattedMarkers', 'Show syntax markers in Formatted', 'Reveal markdown hints in formatted editing surfaces.', settingsDraft.settings.showFormattedMarkers)}
      <label class="settings-row">
        <div><strong>Editor width</strong><span>Measure of the writing column.</span></div>
        <div class="settings-slider">
          <input type="range" min="58" max="96" step="1" value="${settingsDraft.settings.editorWidth}" data-settings-field="editorWidth">
          <output>${settingsDraft.settings.editorWidth} ch</output>
        </div>
      </label>
      ${settingsToggleRow('softWrap', 'Soft wrap', 'Wrap long raw markdown lines to the editor width.', settingsDraft.rawOptions.softWrap)}
      ${settingsToggleRow('lineNumbers', 'Line numbers', 'Show a source gutter in Raw mode.', settingsDraft.rawOptions.lineNumbers)}
      ${settingsToggleRow('hideMarkdownMarkers', 'Hide markers', 'Hide markdown marker glyphs in Raw and Split source without changing source text.', settingsDraft.rawOptions.hideMarkdownMarkers)}
      ${settingsToggleRow('formatOnSave', 'Format on save', 'Normalize markdown spacing and block boundaries when saving.', settingsDraft.settings.formatOnSave)}
    </div>
  `
  content.querySelector('[data-settings-field="defaultMode"]').addEventListener('change', (event) => {
    settingsDraft.settings.defaultMode = event.target.value
  })
  const width = content.querySelector('[data-settings-field="editorWidth"]')
  width.addEventListener('input', () => {
    settingsDraft.settings.editorWidth = Number(width.value)
    content.querySelector('output').textContent = `${width.value} ch`
  })
  for (const field of ['softWrap', 'lineNumbers', 'hideMarkdownMarkers']) {
    content.querySelector(`[data-settings-field="${field}"]`).addEventListener('change', (event) => {
      settingsDraft.rawOptions[field] = event.target.checked
    })
  }
  for (const field of ['showFormattedMarkers', 'formatOnSave']) {
    content.querySelector(`[data-settings-field="${field}"]`).addEventListener('change', (event) => {
      settingsDraft.settings[field] = event.target.checked
    })
  }
}

function renderUnavailableSettings(content, title) {
  content.innerHTML = settingsHeader(title, 'This settings area is part of the product direction but is not backed in this build.') + `
    <div class="settings-rows">
      <div class="settings-row"><div><strong>Not available yet</strong><span>This section is disabled until its native behavior exists.</span></div></div>
    </div>
  `
}

function settingsTitleForTab(tab) {
  return {
    markdown: 'Markdown flavour',
    sync: 'Sync & Git',
    extensions: 'Extensions',
  }[tab] || 'Settings'
}

function renderAppearanceSettings(content) {
  content.innerHTML = settingsHeader('Appearance', 'Control the reading surface and code typography.') + `
    <div class="settings-rows">
      <div class="settings-row">
        <div><strong>Theme</strong><span>Use a light or dark application chrome.</span></div>
        <div class="settings-segmented" role="group" aria-label="Theme">
          <button type="button" data-settings-theme="light">Light</button>
          <button type="button" data-settings-theme="dark">Dark</button>
        </div>
      </div>
      <label class="settings-row">
        <div><strong>Document font</strong><span>Font used for rendered markdown prose.</span></div>
        <select data-settings-field="documentFont">
          ${fontOptions(settingsDraft.settings.documentFont)}
        </select>
      </label>
      <label class="settings-row">
        <div><strong>Document font size</strong><span>Size of rendered document text.</span></div>
        <div class="settings-slider">
          <input type="range" min="13" max="20" step="0.5" value="${settingsDraft.settings.documentFontSize}" data-settings-field="documentFontSize">
          <output>${settingsDraft.settings.documentFontSize}px</output>
        </div>
      </label>
      <label class="settings-row">
        <div><strong>Code font</strong><span>Font used in raw mode, source panes, and code blocks.</span></div>
        <select data-settings-field="codeFont">
          ${fontOptions(settingsDraft.settings.codeFont)}
        </select>
      </label>
      ${settingsToggleRow('codeLigatures', 'Code ligatures', 'Allow programming ligatures when the selected code font supports them.', settingsDraft.settings.codeLigatures)}
    </div>
  `
  refreshThemeDraftButtons(content)
  content.querySelectorAll('[data-settings-theme]').forEach((button) => {
    button.addEventListener('click', () => {
      settingsDraft.settings.theme = button.dataset.settingsTheme
      refreshThemeDraftButtons(content)
    })
  })
  const fontSize = content.querySelector('[data-settings-field="documentFontSize"]')
  fontSize.addEventListener('input', () => {
    settingsDraft.settings.documentFontSize = Number(fontSize.value)
    content.querySelector('output').textContent = `${fontSize.value}px`
  })
  content.querySelector('[data-settings-field="documentFont"]').addEventListener('change', (event) => {
    settingsDraft.settings.documentFont = event.target.value
  })
  content.querySelector('[data-settings-field="codeFont"]').addEventListener('change', (event) => {
    settingsDraft.settings.codeFont = event.target.value
  })
  content.querySelector('[data-settings-field="codeLigatures"]').addEventListener('change', (event) => {
    settingsDraft.settings.codeLigatures = event.target.checked
  })
}

function fontOptions(selected) {
  return state.systemFonts.map((font) =>
    `<option value="${font}" ${selected === font ? 'selected' : ''}>${font}</option>`
  ).join('')
}

function renderShortcutSettings(content) {
  content.innerHTML = settingsHeader('Shortcuts', 'Implemented keyboard commands available in this build.') + `
    <div class="settings-rows shortcut-settings">
      ${shortcutRow('Toggle Raw', 'Cmd Shift R')}
      ${shortcutRow('Open Split', 'Cmd Shift S')}
      ${shortcutRow('New document', 'Cmd N')}
      ${shortcutRow('Open file', 'Cmd O')}
      ${shortcutRow('Save', 'Cmd S')}
      ${shortcutRow('Bold', 'Cmd B')}
      ${shortcutRow('Link', 'Cmd K')}
    </div>
  `
}

function settingsHeader(title, description) {
  return `<h1 id="settings-title">${title}</h1><p>${description}</p>`
}

function settingsToggleRow(field, label, description, checked) {
  return `
    <label class="settings-row">
      <div><strong>${label}</strong><span>${description}</span></div>
      <input class="settings-toggle" type="checkbox" data-settings-field="${field}" ${checked ? 'checked' : ''}>
    </label>
  `
}

function shortcutRow(label, shortcut) {
  return `<div class="settings-row"><div><strong>${label}</strong></div><kbd>${shortcut}</kbd></div>`
}

function refreshThemeDraftButtons(content) {
  content.querySelectorAll('[data-settings-theme]').forEach((button) => {
    button.classList.toggle('active', button.dataset.settingsTheme === settingsDraft.settings.theme)
  })
}

function onSplitEdited() {
  const md = els.splitSource.value
  refreshSplitSourceHighlight()
  refreshSplitPreview(md)
  markEdited(md)
}

function refreshSplitSourceHighlight() {
  els.splitSourceHighlight.innerHTML = `${highlightMarkdownSource(els.splitSource.value, state.rawOptions)}\n`
}

function syncSplitSourceScroll() {
  els.splitSourceHighlight.scrollTop = els.splitSource.scrollTop
  els.splitSourceHighlight.scrollLeft = els.splitSource.scrollLeft
  syncSplitScroll(els.splitSource, els.splitPreview)
}

function syncSplitPreviewScroll() {
  syncSplitScroll(els.splitPreview, els.splitSource)
  els.splitSourceHighlight.scrollTop = els.splitSource.scrollTop
}

function syncSplitScroll(source, target) {
  if (syncingSplitScroll || state.mode !== 'split') return
  const sourceScrollable = source.scrollHeight - source.clientHeight
  const targetScrollable = target.scrollHeight - target.clientHeight
  if (sourceScrollable <= 0 || targetScrollable <= 0) return
  syncingSplitScroll = true
  target.scrollTop = (source.scrollTop / sourceScrollable) * targetScrollable
  requestAnimationFrame(() => {
    syncingSplitScroll = false
  })
}

async function refreshSplitPreview(md) {
  els.splitPreview.replaceChildren(...await renderMarkdownPreview(md))
  await resolveImageAssets(els.splitPreview)
}

async function renderMarkdownPreview(md) {
  const nodes = []
  const lines = md.split('\n')
  let codeFenceIndex = -1
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const heading = line.match(/^(#{1,6})\s+(.+)$/)
    if (heading) {
      const h = document.createElement(`h${heading[1].length}`)
      h.textContent = heading[2]
      nodes.push(h)
    } else if (/^```/.test(line)) {
      codeFenceIndex++
      const language = line.replace(/^```\s*/, '').trim().split(/\s+/)[0] || ''
      const body = []
      i++
      while (i < lines.length && !/^```/.test(lines[i])) body.push(lines[i++])
      if (normalizeLanguage(language) === 'mermaid') {
        const diagram = document.createElement('div')
        diagram.className = 'mermaid-render'
        diagram.dataset.language = 'mermaid'
        diagram.innerHTML = await renderMermaidDiagram(body.join('\n'))
        nodes.push(diagram)
        continue
      }
      nodes.push(codeBlockElement(body.join('\n'), language, codeFenceIndex))
    } else if (/^\s*[-*+]\s+/.test(line) || /^\s*\d+\.\s+/.test(line)) {
      const p = document.createElement('p')
      p.textContent = line.replace(/^\s*(?:[-*+]|\d+\.)\s+/, '• ')
      nodes.push(p)
    } else if (/^>\s?/.test(line)) {
      const q = document.createElement('blockquote')
      q.textContent = line.replace(/^>\s?/, '')
      nodes.push(q)
    } else if (/^---+$/.test(line.trim())) {
      nodes.push(document.createElement('hr'))
    } else if (line.trim()) {
      const p = document.createElement('p')
      p.append(...inlineMarkdownNodes(line))
      nodes.push(p)
    }
  }
  return nodes.length ? nodes : [document.createElement('p')]
}

function codeBlockElement(source, language = '', fenceIndex = null) {
  const normalized = normalizeLanguage(language)
  const figure = document.createElement('figure')
  figure.className = 'code-block-shell'
  figure.dataset.language = normalized || 'text'
  if (Number.isInteger(fenceIndex)) figure.dataset.codeFenceIndex = String(fenceIndex)

  const header = document.createElement('figcaption')
  header.className = 'code-block-header'
  const label = document.createElement('span')
  label.className = 'code-block-language'
  label.textContent = normalized || 'text'
  const copy = document.createElement('button')
  copy.className = 'code-block-copy'
  copy.type = 'button'
  copy.textContent = 'Copy'
  copy.addEventListener('click', () => navigator.clipboard?.writeText(source))
  header.append(label)
  if (Number.isInteger(fenceIndex)) header.append(codeLanguageTool(fenceIndex, normalized || 'text'))
  header.append(copy)
  if (Number.isInteger(fenceIndex)) {
    figure.addEventListener('contextmenu', (event) => {
      event.preventDefault()
      openCodeAssistant({ editFenceIndex: fenceIndex, language: normalized || 'text' })
    })
  }

  const pre = document.createElement('pre')
  const code = document.createElement('code')
  code.dataset.language = language
  code.className = language ? `language-${normalized}` : ''
  code.innerHTML = highlightCode(source, language)
  pre.append(code)
  figure.append(header, pre)
  return figure
}

function codeLanguageTool(fenceIndex, language) {
  const button = document.createElement('button')
  button.className = 'code-language-tool'
  button.type = 'button'
  button.dataset.codeLanguageTool = 'true'
  button.textContent = 'Language'
  button.title = `Change code language (${language})`
  button.addEventListener('click', () => openCodeAssistant({ editFenceIndex: fenceIndex, language }))
  return button
}

function inlineMarkdownNodes(text) {
  const nodes = []
  // Images must precede the link alternative: `![alt](src)` otherwise matches
  // as a link and renders the leading `!` as stray text.
  const pattern = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*|!\[[^\]]*\]\([^)]+\)|<img\b[^>]*>|\[[^\]]+\]\([^)]+\))/g
  let cursor = 0
  let match = pattern.exec(text)
  while (match) {
    if (match.index > cursor) nodes.push(document.createTextNode(text.slice(cursor, match.index)))
    nodes.push(inlineMarkdownNode(match[0]))
    cursor = match.index + match[0].length
    match = pattern.exec(text)
  }
  if (cursor < text.length) nodes.push(document.createTextNode(text.slice(cursor)))
  return nodes
}

// --- image block model ---
//
// Images exist in two on-disk forms. `![alt](path)` is the portable default;
// a sized image becomes `<img src alt width>` because CommonMark has no size
// syntax. Clearing the width returns the image to the portable form, so the
// HTML form only ever appears where it carries information markdown cannot.


// Preset widths in CSS pixels; "Original" clears the width entirely.








// handleDocumentLinkClick sends document links to the user's browser.
//
// Refusing a link is an autonomous security decision taken against untrusted
// document content, with no user interaction, and it used to leave no trace at
// all — not even a console line. A document probing for a `javascript:` URL
// against bindings that expose arbitrary file read and write is precisely the
// event a maintainer needs after the fact.
//
// Recorded once per distinct href rather than per render, because the same
// refused link is rendered again and again. Measured, rather than assumed:
// typing does NOT rebuild the preview anchors (10 simulated edits produced one
// record), but every switch back into split mode does — five round trips
// produced six. Recording unconditionally would let a document the app has
// already judged hostile flood a trimmed log and evict every other event in it,
// turning an audit record into a way to erase one. The cap bounds the same
// attack carried out with many distinct hrefs instead of one repeated href.
const REFUSED_LINK_RECORD_CAP = 32
const refusedLinks = new Set()

function recordRefusedLink(href) {
  const key = String(href ?? '')
  if (refusedLinks.has(key) || refusedLinks.size >= REFUSED_LINK_RECORD_CAP) return
  refusedLinks.add(key)
  bridge.recordEvent('link.refused', { href: key })
}

// The href was already filtered by safeLinkHref when the anchor was built, but
// it is re-checked here rather than trusted: this handler is bound to the whole
// document, so it must be safe for any anchor that reaches it, not only the
// ones this module created.
function handleDocumentLinkClick(event) {
  const anchor = event.target.closest?.('a[href]')
  if (!anchor) return
  const raw = anchor.getAttribute('href')
  const safe = safeLinkHref(raw)
  event.preventDefault()
  if (safe === null) {
    // This handler is bound document-wide, so it must be safe for any anchor
    // that reaches it — including one this module never built, which therefore
    // never passed through the render-time check above.
    recordRefusedLink(raw)
    return
  }
  bridge.openExternalURL(safe)?.catch?.((error) => {
    console.warn('bridge: refused to open link', error)
    bridge.recordEvent('link.open-failed', { error: String(error?.message ?? error) })
  })
}


function inlineMarkdownNode(token) {
  if (token.startsWith('![')) {
    const image = document.createElement('img')
    const parsed = token.match(/^!\[([^\]]*)\]\(([^)\s]+)/)
    image.alt = parsed?.[1] ?? ''
    image.setAttribute('src', parsed?.[2] ?? '')
    return image
  }
  if (token.startsWith('<img')) {
    // Only the attributes this editor emits are carried over. Parsing the raw
    // tag would let a document supply event handlers (onerror=...) that run
    // against a webview holding the native file bindings.
    const image = document.createElement('img')
    image.setAttribute('src', htmlImageAttribute(token, 'src'))
    image.alt = htmlImageAttribute(token, 'alt')
    const width = htmlImageAttribute(token, 'width')
    if (width) image.setAttribute('width', width)
    return image
  }
  if (token.startsWith('`')) {
    const code = document.createElement('code')
    code.textContent = token.slice(1, -1)
    return code
  }
  if (token.startsWith('**')) {
    const strong = document.createElement('strong')
    strong.textContent = token.slice(2, -2)
    return strong
  }
  if (token.startsWith('*')) {
    const em = document.createElement('em')
    em.textContent = token.slice(1, -1)
    return em
  }
  const link = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/)
  if (link) {
    const anchor = document.createElement('a')
    anchor.textContent = link[1]
    // Only navigable schemes. An opened document is untrusted input, and this
    // webview holds the native bindings — SaveDocument writes any path,
    // OpenRecentDocument reads any path. A `javascript:` href here executed
    // attacker script in the app origin on a single click, which is an
    // ordinary action in a markdown viewer.
    const safe = safeLinkHref(link[2])
    if (safe !== null) anchor.href = safe
    else {
      anchor.dataset.blockedHref = 'true'
      recordRefusedLink(link[2])
    }
    return anchor
  }
  return document.createTextNode(token)
}

function toggleInsertPopover() {
  const existing = els.insertPopoverRoot.querySelector('[data-insert-menu]')
  if (existing) {
    existing.remove()
    return
  }
  const menu = document.createElement('div')
  menu.className = 'insert-popover'
  menu.dataset.insertMenu = 'true'
  menu.innerHTML = `
    <div class="popover-caption">Insert Block</div>
    ${insertMenuButton('code-block', '{ }', 'Code block', '⌘C')}
    ${insertMenuButton('table', '|-|', 'Table', '⌘T')}
    ${insertMenuButton('mermaid', '⇄', 'Mermaid diagram', '⌘D')}
    <div class="popover-divider"></div>
    ${insertMenuButton('hr', '—', 'Horizontal rule', '')}
    ${insertMenuButton('task-list', '☐', 'Task list', '')}
  `
  menu.querySelectorAll('[data-insert-command]').forEach((button) => {
    button.addEventListener('click', async () => {
      if (button.dataset.insertCommand === 'code-block') openCodeAssistant()
      else if (button.dataset.insertCommand === 'mermaid') openDiagramAssistant()
      else await runCommand(button.dataset.insertCommand)
      menu.remove()
    })
  })
  els.insertPopoverRoot.replaceChildren(menu)
}

function toggleExportMenu() {
  const existing = els.exportRoot.querySelector('[data-export-menu]')
  if (existing) {
    existing.remove()
    return
  }
  const menu = document.createElement('div')
  menu.className = 'insert-popover export-popover'
  menu.dataset.exportMenu = 'true'
  menu.innerHTML = `
    <div class="popover-caption">Export</div>
    ${exportMenuButton('print', '⌘P', 'Print')}
    ${exportMenuButton('pdf', 'PDF', 'Export to PDF')}
  `
  menu.querySelectorAll('[data-export-action]').forEach((button) => {
    button.addEventListener('click', () => printDocument(button.dataset.exportAction))
  })
  els.exportRoot.replaceChildren(menu)
}

function closeExportMenu() {
  els.exportRoot.replaceChildren()
}

async function printDocument(action = 'print') {
  await persistCurrentEditorText()
  const doc = activeDoc()
  els.printRoot.replaceChildren(...await renderMarkdownPreview(doc?.markdown ?? ''))
  // Inline before printing: the print surface must be self-contained, and the
  // OS print/PDF path cannot resolve document-relative asset paths.
  await resolveImageAssets(els.printRoot)
  document.body.dataset.lastExportAction = action
  if (action === 'pdf') document.body.dataset.pdfExportVia = 'print-dialog'
  else delete document.body.dataset.pdfExportVia
  closeExportMenu()
  window.print()
}

// The File menu.
//
// File left the ribbon because it did not belong there: every other tab changes
// what the ribbon offers for the document you are editing, and File acts on the
// document as a whole. The consequence was that the application opened showing
// commands you use once a session while the formatting controls you use
// constantly sat behind another tab.
//
// It is a menu rather than a tab so that reaching it costs you nothing: the
// ribbon stays on whatever you were doing. Every command here is also in the
// native menu bar, which is where a Mac user looks first; this is the affordance
// for someone who does not.
function fileMenuItem(action, label, shortcut) {
  return `<button type="button" role="menuitem" data-file-menu-action="${action}">` +
    `<span>${label}</span><kbd>${shortcut}</kbd></button>`
}

function toggleFileMenu() {
  if (els.fileMenuRoot.firstChild) return closeFileMenu()
  const menu = document.createElement('div')
  menu.className = 'native-menu'
  menu.setAttribute('role', 'menu')
  menu.dataset.fileMenu = 'true'
  menu.innerHTML = [
    fileMenuItem('new', 'New', '\u2318N'),
    fileMenuItem('open', 'Open…', '\u2318O'),
    '<div class="native-menu-separator"></div>',
    fileMenuItem('save', 'Save', '\u2318S'),
    fileMenuItem('save-as', 'Save As…', '\u21e7\u2318S'),
    '<div class="native-menu-separator"></div>',
    fileMenuItem('print', 'Print…', '\u2318P'),
    fileMenuItem('pdf', 'Export as PDF…', ''),
  ].join('')
  menu.querySelectorAll('[data-file-menu-action]').forEach((button) => {
    button.addEventListener('click', () => runFileMenuAction(button.dataset.fileMenuAction))
  })
  els.fileMenuRoot.replaceChildren(menu)
  els.btnFileMenu.setAttribute('aria-expanded', 'true')
  // A menu closes on the next click anywhere, and on Escape, because that is
  // what a menu does. Registered once the menu exists so the opening click does
  // not immediately close it.
  setTimeout(() => {
    document.addEventListener('click', closeFileMenuOnOutsideClick, { once: true })
  }, 0)
}

function closeFileMenu() {
  els.fileMenuRoot.replaceChildren()
  els.btnFileMenu.setAttribute('aria-expanded', 'false')
}

function closeFileMenuOnOutsideClick(event) {
  if (event.target.closest('#btn-file-menu')) return
  closeFileMenu()
}

function runFileMenuAction(action) {
  closeFileMenu()
  if (action === 'print' || action === 'pdf') return printDocument(action)
  const run = { new: newDocument, open: openDocument, save, 'save-as': saveAs }[action]
  return run?.()
}

function exportMenuButton(action, glyph, label) {
  return `<button type="button" data-export-action="${action}"><span>${glyph}</span><strong>${label}</strong><kbd></kbd></button>`
}

function openHelpPanel(section = 'markdown') {
  const panel = document.createElement('div')
  panel.className = 'settings-scrim help-scrim'
  panel.dataset.helpPanel = section
  panel.innerHTML = `
    <section class="help-panel" role="dialog" aria-modal="true" aria-labelledby="help-title">
      <h1 id="help-title">${section === 'shortcuts' ? 'Keyboard Shortcuts' : 'Markdown Help'}</h1>
      <div class="help-grid">
        ${helpRows(section).map(([key, value]) => `<div><kbd>${key}</kbd><span>${value}</span></div>`).join('')}
      </div>
      <footer class="settings-footer">
        <button class="primary" type="button" data-help-close>Done</button>
      </footer>
    </section>
  `
  panel.querySelector('[data-help-close]').addEventListener('click', closeHelpPanel)
  els.helpRoot.replaceChildren(panel)
  focusFirstControl(panel)
}

function closeHelpPanel() {
  els.helpRoot.replaceChildren()
}

function closeTransientSurfaces() {
  closeExportMenu()
  closeHelpPanel()
  closeSettings()
  closeCodeAssistant()
  closeDiagramAssistant()
  els.insertPopoverRoot.replaceChildren()
}

function helpRows(section) {
  if (section === 'shortcuts') {
    return [['⌘⇧R', 'Toggle Raw mode'], ['⌘⇧S', 'Toggle Split mode'], ['⌘B', 'Bold'], ['⌘I', 'Italic']]
  }
  return [['#', 'Heading'], ['**text**', 'Bold'], ['[text](url)', 'Link'], ['```lang', 'Code block'], ['```mermaid', 'Mermaid Diagram']]
}

function insertMenuButton(command, glyph, label, shortcut) {
  return `<button type="button" data-insert-command="${command}"><span>${glyph}</span><strong>${label}</strong><kbd>${shortcut}</kbd></button>`
}

function openCodeAssistant({ editFenceIndex = null, language = 'javascript' } = {}) {
  editingCodeFenceIndex = editFenceIndex
  selectedCodeLanguage = normalizeLanguage(language || 'javascript') || 'javascript'
  renderCodeAssistant()
}

function closeCodeAssistant() {
  editingCodeFenceIndex = null
  els.codeAssistantRoot.replaceChildren()
}

async function insertSelectedCodeBlock() {
  const language = selectedCodeLanguage || 'text'
  await persistCurrentEditorText()
  startEditing()
  const doc = activeDoc()
  doc.markdown = Number.isInteger(editingCodeFenceIndex)
    ? rewriteCodeFenceLanguage(doc.markdown, editingCodeFenceIndex, language)
    : appendBlock(doc.markdown, `\`\`\`${language}\n${codeStarters[language] ?? codeStarters.text}\n\`\`\``)
  await mountMarkdown(doc.markdown)
  markEdited(doc.markdown)
  closeCodeAssistant()
}

function renderCodeAssistant() {
  const editing = Number.isInteger(editingCodeFenceIndex)
  const modal = document.createElement('div')
  modal.className = 'settings-scrim code-scrim'
  modal.dataset.codeAssistant = 'true'
  if (editing) modal.dataset.codeEditIndex = String(editingCodeFenceIndex)
  modal.innerHTML = `
    <section class="code-dialog" role="dialog" aria-modal="true" aria-labelledby="code-title">
      <div class="code-content">
        <h1 id="code-title">${editing ? 'Code Block Language' : 'Code Block'}</h1>
        <p>${editing ? 'Change the language used to highlight this fenced block.' : 'Choose the language for syntax highlighting.'}</p>
        <label class="settings-row code-language-row">
          <div><strong>Language</strong><span>Stored in the fenced code info string.</span></div>
          <select data-code-language>
            ${codeLanguages.map(([value, label]) => `<option value="${value}" ${selectedCodeLanguage === value ? 'selected' : ''}>${label}</option>`).join('')}
          </select>
        </label>
      </div>
      <footer class="settings-footer">
        <button type="button" data-code-action="cancel">Cancel</button>
        <button class="primary" type="button" data-code-action="${editing ? 'apply' : 'insert'}">${editing ? 'Apply language' : 'Insert code block'}</button>
      </footer>
    </section>
  `
  modal.querySelector('[data-code-language]').addEventListener('change', (event) => {
    selectedCodeLanguage = event.target.value
  })
  modal.querySelector('[data-code-action="cancel"]').addEventListener('click', closeCodeAssistant)
  modal.querySelector(`[data-code-action="${editing ? 'apply' : 'insert'}"]`).addEventListener('click', insertSelectedCodeBlock)
  els.codeAssistantRoot.replaceChildren(modal)
  focusFirstControl(modal)
}

function openDiagramAssistant({ editFenceIndex = null } = {}) {
  editingDiagramFenceIndex = editFenceIndex
  selectedDiagramType = 'flowchart'
  renderDiagramAssistant()
}

function closeDiagramAssistant() {
  editingDiagramFenceIndex = null
  els.diagramAssistantRoot.replaceChildren()
}

async function insertSelectedDiagram() {
  await persistCurrentEditorText()
  startEditing()
  const doc = activeDoc()
  const source = currentDiagramSource()
  doc.markdown = Number.isInteger(editingDiagramFenceIndex)
    ? rewriteMermaidFenceSource(doc.markdown, editingDiagramFenceIndex, source)
    : appendBlock(doc.markdown, `\`\`\`mermaid\n${source}\n\`\`\``)
  await mountMarkdown(doc.markdown)
  markEdited(doc.markdown)
  closeDiagramAssistant()
}


function renderDiagramAssistant() {
  const editing = Number.isInteger(editingDiagramFenceIndex)
  const modal = document.createElement('div')
  modal.className = 'settings-scrim diagram-scrim'
  modal.dataset.diagramAssistant = 'true'
  if (editing) modal.dataset.diagramEditIndex = String(editingDiagramFenceIndex)
  modal.innerHTML = `
    <section class="diagram-dialog" role="dialog" aria-modal="true" aria-labelledby="diagram-title">
      <div class="diagram-content">
        <h1 id="diagram-title">${editing ? 'Edit Mermaid Diagram' : 'Mermaid Diagram'}</h1>
        <p>${editing ? 'Apply a guided replacement to the selected diagram.' : 'Choose the starter diagram that best matches what you want to draw.'}</p>
        <div class="diagram-type-grid">
          ${Object.entries(diagramTemplates).map(([type, template]) => diagramTypeButton(type, template)).join('')}
        </div>
        <div class="diagram-builder">
          <div class="diagram-fields" data-diagram-fields>
            ${diagramFieldsMarkup()}
          </div>
          <div class="diagram-preview" data-diagram-preview aria-label="Diagram preview"></div>
        </div>
      </div>
      <footer class="settings-footer">
        <button type="button" data-diagram-action="cancel">Cancel</button>
        <button class="primary" type="button" data-diagram-action="insert">${editing ? 'Apply diagram' : 'Insert diagram'}</button>
      </footer>
    </section>
  `
  modal.querySelectorAll('[data-diagram-type]').forEach((button) => {
    button.addEventListener('click', () => {
      selectedDiagramType = button.dataset.diagramType
      refreshDiagramSelection(modal)
      modal.querySelector('[data-diagram-fields]').innerHTML = diagramFieldsMarkup()
      bindDiagramFields(modal)
      refreshDiagramPreview(modal)
    })
  })
  modal.querySelector('[data-diagram-action="cancel"]').addEventListener('click', closeDiagramAssistant)
  modal.querySelector('[data-diagram-action="insert"]').addEventListener('click', insertSelectedDiagram)
  els.diagramAssistantRoot.replaceChildren(modal)
  refreshDiagramSelection(modal)
  bindDiagramFields(modal)
  refreshDiagramPreview(modal)
  focusFirstControl(modal)
}

function diagramTypeButton(type, template) {
  return `
    <button type="button" data-diagram-type="${type}">
      <strong>${template.label}</strong>
      <span>${template.description}</span>
    </button>
  `
}

function refreshDiagramSelection(modal) {
  modal.querySelectorAll('[data-diagram-type]').forEach((button) => {
    button.classList.toggle('active', button.dataset.diagramType === selectedDiagramType)
  })
}

function diagramFieldsMarkup() {
  const template = diagramTemplates[selectedDiagramType] ?? diagramTemplates.flowchart
  const draft = diagramDrafts[selectedDiagramType] ?? diagramDrafts.flowchart
  return template.fields.map(([key, label]) => `
    <label>
      <span>${label}</span>
      <input data-diagram-field="${key}" value="${escapeHtml(draft[key] ?? '')}" />
    </label>
  `).join('')
}

function bindDiagramFields(modal) {
  modal.querySelectorAll('[data-diagram-field]').forEach((input) => {
    input.addEventListener('input', () => {
      const draft = diagramDrafts[selectedDiagramType] ?? diagramDrafts.flowchart
      draft[input.dataset.diagramField] = input.value
      refreshDiagramPreview(modal)
    })
  })
}

async function refreshDiagramPreview(modal) {
  const preview = modal.querySelector('[data-diagram-preview]')
  if (!preview) return
  preview.innerHTML = await renderMermaidDiagram(currentDiagramSource())
}

function currentDiagramSource() {
  const template = diagramTemplates[selectedDiagramType] ?? diagramTemplates.flowchart
  const draft = diagramDrafts[selectedDiagramType] ?? diagramDrafts.flowchart
  return template.body(draft)
}

async function pasteMarkdown() {
  // The optional chaining guards clipboard ABSENCE. It does not guard refusal,
  // and refusal is the common case: macOS denies a clipboard read that has no
  // user gesture behind it, and the read then REJECTS. That rejection escaped
  // this function, so the button did nothing at all — no paste, and not even
  // the empty-clipboard fallback below.
  //
  // Invisible to the suite because the clipboard is stubbed there, so the real
  // path had never run. Found by walking the UI in a native host.
  let text
  try {
    text = await navigator.clipboard?.readText?.()
  } catch (error) {
    // Contained, but never silent. A user who clicks Paste and sees nothing
    // has no way to tell a denied clipboard from a broken button.
    bridge.recordEvent('clipboard.read-refused', { error: String(error?.message ?? error) })
  }

  if (text) {
    await setMarkdown(text)
    markEdited(text)
    return
  }
  startEditing()
  syncActiveState()
}

async function applyTemplate(name) {
  const md = templates[name]
  if (!md) return
  await setMarkdown(md)
  markEdited(md)
}

// --- dirty tracking ---

function onEdited(md) {
  const doc = activeDoc()
  if (!doc) return
  markEdited(md)
}

function markEdited(md) {
  const doc = activeDoc()
  doc.markdown = md
  doc.started = true
  doc.dirty = md !== doc.savedText
  syncActiveState()
  pushDirtyState()
  clearTimeout(pushTimer)
  pushTimer = setTimeout(() => {
    bridge.updateContent(md)
  }, 300)
}

// Go must never infer which document a write targets. Push the whole tab set
// — path, content and dirty flag per tab — so the close guard saves each one
// to its own file. Previously only a single boolean crossed the bridge, and Go
// paired it with whatever path it had last opened, which wrote one tab's text
// over another tab's file and discarded dirty background tabs entirely.
function pushDocumentState() {
  bridge.syncDocuments(state.docs.map((doc) => ({
    path: doc.path ?? '',
    content: doc.markdown ?? '',
    dirty: Boolean(doc.dirty),
    active: doc.id === state.activeDocId,
  })))
}

function pushDirtyState() {
  pushDocumentState()
  if (state.dirty !== lastPushedDirty) {
    bridge.setDirty(state.dirty)
    lastPushedDirty = state.dirty
  }
}

function cancelPendingPush() {
  clearTimeout(pushTimer)
  pushTimer = null
}

function wire() {
  document.addEventListener('selectionchange', () => {
    currentEditorContext()
  })

  els.emptyStart.addEventListener('click', () => {
    startEditing()
    syncActiveState()
  })
  els.emptyPaste.addEventListener('click', pasteMarkdown)
  els.btnToggle.addEventListener('click', toggleMode)
  els.btnModeFormatted.addEventListener('click', () => {
    if (state.mode !== 'wysiwyg') setMode('wysiwyg')
  })
  els.btnModeRaw.addEventListener('click', () => {
    if (state.mode !== 'raw') setMode('raw')
  })
  // Selects, like its peers. It used to toggle back to Formatted, which is what
  // made Split read as a modifier on a mode rather than a mode of its own.
  els.btnSplit.addEventListener('click', () => {
    if (state.mode !== 'split') setMode('split')
  })
  els.splitSource.addEventListener('input', onSplitEdited)
  els.splitSource.addEventListener('scroll', syncSplitSourceScroll)
  els.splitPreview.addEventListener('scroll', syncSplitPreviewScroll)
  els.btnInsertMenu.addEventListener('click', toggleInsertPopover)
  els.btnFileMenu.addEventListener('click', toggleFileMenu)
  els.btnSettings.addEventListener('click', openSettings)
  els.workspace.addEventListener('click', (event) => {
    const button = event.target.closest('[data-panel-toggle]')
    if (button) toggleSidePanel(button.dataset.panelToggle)
  })
  // Every link in a rendered document is handled here, once, at the document
  // level. A link left to its default action navigates THIS window — a
  // chrome-less desktop window with no address bar and no back button — so a
  // single click on a document's link replaces the application with a remote
  // page and the only way out is quitting. Delegating from the document covers
  // the split preview, the print surface and anything added later, rather than
  // leaving each new render surface to remember on its own.
  document.addEventListener('click', handleDocumentLinkClick)
  els.wysiwyg.addEventListener('click', handleRenderedBlockSelection)
  wireZoomControl()
  document.querySelectorAll('[data-export-toggle]').forEach((button) => {
    button.addEventListener('click', toggleExportMenu)
  })
  document.querySelectorAll('[data-help-toggle]').forEach((button) => {
    button.addEventListener('click', () => openHelpPanel('markdown'))
  })
  document.querySelectorAll('[data-shortcuts-toggle]').forEach((button) => {
    button.addEventListener('click', () => openHelpPanel('shortcuts'))
  })
  // Wired by attribute, not by id, because these actions now appear in more
  // than one place: the File ribbon tab, the rail's New document button, and
  // the empty state's Open a file. An id can only ever name one element, which
  // is why the ribbon had no file operations at all -- Save and Save As existed
  // as HIDDEN buttons outside it, present only to be a click target.
  const fileActions = { new: newDocument, open: openDocument, save, 'save-as': saveAs }
  document.querySelectorAll('[data-file-action]').forEach((button) => {
    const run = fileActions[button.dataset.fileAction]
    if (run) button.addEventListener('click', run)
  })
  els.btnCloseTab.addEventListener('click', closeActiveTab)
  els.blockStyle.addEventListener('change', (e) => {
    applyBlockStyle(e.target.value)
    e.target.value = 'normal'
  })
  els.ribbonTabs.forEach((tab) => {
    tab.addEventListener('click', () => activateRibbonTab(tab.dataset.ribbonTab))
  })
  els.outlineTabs.forEach((tab) => {
    tab.addEventListener('click', () => {
      state.outlineTab = tab.dataset.outlineTab
      refreshOutlinePanel(activeDoc())
    })
  })
  els.fileSearchInput.addEventListener('input', (e) => {
    state.fileFilter = e.target.value
    refreshFileRail()
  })
  document.querySelectorAll('[data-template]').forEach((button) => {
    button.addEventListener('click', () => applyTemplate(button.dataset.template))
  })
  document.querySelectorAll('[data-command]').forEach((button) => {
    button.addEventListener('pointerdown', (event) => event.preventDefault())
    button.addEventListener('click', () => {
      if (button.dataset.command === 'code-block') openCodeAssistant()
      else if (button.dataset.command === 'mermaid') openDiagramAssistant()
      else runCommand(button.dataset.command)
    })
  })
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      closeTransientSurfaces()
      return
    }
    const mod = e.ctrlKey || e.metaKey
    if (!mod) return
    const key = e.key.toLowerCase()
    if (key === 'r' && e.shiftKey) {
      e.preventDefault()
      toggleMode()
    } else if (key === 'e') {
      e.preventDefault()
      toggleMode()
    } else if (key === 'n') {
      e.preventDefault()
      newDocument()
    } else if (key === 'o') {
      e.preventDefault()
      openDocument()
    } else if (key === 'w') {
      e.preventDefault()
      closeActiveTab()
    } else if (key === 's' && e.shiftKey) {
      e.preventDefault()
      toggleSplit()
    } else if (key === 's') {
      e.preventDefault()
      save()
    } else if (key === 'b') {
      e.preventDefault()
      runCommand('bold')
    } else if (key === 'i') {
      e.preventDefault()
      runCommand('italic')
    } else if (key === 'k') {
      e.preventDefault()
      runCommand('link')
    } else if (key === '`') {
      e.preventDefault()
      runCommand('inline-code')
    } else if (key === 'x' && e.shiftKey) {
      e.preventDefault()
      runCommand('strike')
    }
  })
}

async function boot() {
  await loadTheme()
  await loadNativePreferences()
  applyRuntimeSettings()
  suppressBrowserDefaults()
  const first = createDoc()
  state.docs.push(first)
  state.activeDocId = first.id
  await wysiwyg.create(els.wysiwyg, first.markdown, onEdited)
  first.markdown = wysiwyg.getMarkdown()
  first.savedText = first.markdown
  wire()
  syncActiveState()
  pushDirtyState()

  window.__app = {
    ready: true,
    state,
    getMarkdown,
    // The markdown the WYSIWYG editor itself serializes, bypassing the cached
    // doc.markdown. The round-trip corpus MUST use this: getMarkdown() returns
    // the string that was last put in, so a corpus built on it compares a value
    // to itself and cannot fail for the class of defect it exists to catch.
    getEditorMarkdown: () => wysiwyg.getMarkdown(),
    setMarkdown,
    toggleMode,
    setMode,
    toggleSplit,
    openDocument,
    openRecentDocument,
    save,
    saveAs,
    newDocument,
    activateDocument,
    closeActiveTab,
    runCommand,
    printDocument,
    setImageAltText,
    setImageWidth,
    handleDroppedFiles,
    activateRibbonTab,
    applyTemplate,
    pasteMarkdown,
    setRawOption,
    openSettings,
    saveSettings,
    closeSettings,
    toggleSidePanel,
    revealSelectedImage,
    flashStatus,
    debugReplaceRaw: (text) => raw.replaceAll(text),
    debugSimulateEdit: (md) => markEdited(md),
  }

  await consumeFilesHandedOverAtLaunch()
}

// Files macOS handed us, opened once the editor can accept them.
//
// Two arrival paths, because there are two situations. At LAUNCH the file
// reaches Go before this webview exists, so it is held and we ask for it here.
// While the app is already RUNNING there is nothing to wait for, so Go emits
// `file:open` and we listen. Before this, neither was consumed: the bundle
// advertised the association through CFBundleDocumentTypes, macOS routed the
// file in, and the user got an empty document with no hint their file had gone
// (#53).
//
// Failures are contained: a file that will not open must not take the editor
// down with it, and it must not do so silently either.
async function openFileFromOS(path) {
  try {
    await openRecentDocument(path)
  } catch (error) {
    console.warn('bridge: file handed over by the OS could not be opened', error)
    bridge.recordEvent('os-file.open-failed', { error: String(error?.message ?? error) })
  }
}

async function consumeFilesHandedOverAtLaunch() {
  let pending = []
  try {
    pending = (await bridge.frontendReady()) ?? []
  } catch (error) {
    console.warn('bridge: could not ask for files handed over at launch', error)
    bridge.recordEvent('os-file.handover-failed', { error: String(error?.message ?? error) })
    return
  }
  for (const path of pending) await openFileFromOS(path)
  globalThis.drmd?.events?.on?.('file:open', (path) => {
    if (path) openFileFromOS(path)
  })
}

// The recurrence gate for issue #17: catching only the preferences call would
// fix one site. Any boot-time bridge call that rejects must still leave a
// diagnosable failure rather than an unhandled rejection and a blank window.
boot().catch((error) => {
  console.error('boot failed', error)
  bridge.recordEvent('boot.failed', { error: String(error?.message ?? error) })
})
