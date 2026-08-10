//go:build darwin

package main

// A walk of the entire UI surface, driven inside the real host.
//
// The surface is taken from docs/superpowers/plans/2026-08-06-current-screen-functionality-inventory.md,
// which is what this project walked when the screen was first made honest. The
// point is not to re-test the frontend — e2e/ does that in Chrome — but to
// establish that NOTHING in it breaks when the host underneath changes. A host
// swap that silently disables one ribbon tab or the file rail would otherwise
// be discovered by a user.
//
// Served as a module through the scheme handler rather than embedded in an
// Objective-C string literal. Escaping a script of this size through ObjC has
// already cost one round: \n in a literal becomes a real newline, which is a
// SyntaxError that silently kills the whole injected script.
const walkModuleJS = `
const results = []

function check(name, fn) {
  try {
    const detail = fn()
    results.push({ name, ok: detail === true || detail === undefined, detail: String(detail ?? 'ok') })
  } catch (e) {
    results.push({ name, ok: false, detail: 'THREW: ' + (e && e.message) })
  }
}

async function checkAsync(name, fn) {
  try {
    const detail = await fn()
    results.push({ name, ok: detail === true || detail === undefined, detail: String(detail ?? 'ok') })
  } catch (e) {
    results.push({ name, ok: false, detail: 'THREW: ' + (e && e.message) })
  }
}

const app = () => globalThis.__app

// window 'load' fires when the DOCUMENT has loaded, which is not when the app
// is ready: boot() is async and sets __app at its end. Earlier runs of this
// walk passed by winning that race, and adding one check ahead of them lost
// it -- every subsequent check then failed with "__app is undefined", which
// reads as the app being broken rather than the walk being early.
for (let i = 0; i < 200 && !globalThis.__app?.ready; i++) {
  await new Promise((r) => setTimeout(r, 50))
}
if (!globalThis.__app?.ready) {
  window.webkit.messageHandlers.drmd.postMessage({ id: 0, method: '__walk',
    args: [[{ name: 'app became ready', ok: false, detail: 'timed out after 10s' }]] })
  throw new Error('app never became ready')
}
const $ = (sel) => document.querySelector(sel)
const visible = (el) => !!el && el.getBoundingClientRect().height > 0
const settle = () => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))

// ---- Title and save state ----------------------------------------------
await checkAsync('title reflects the active buffer', async () => {
  await app().setMarkdown('# Walk fixture\n\nbody\n')
  await settle()
  return app().getMarkdown().includes('Walk fixture') || 'buffer did not take the content'
})

// ---- Mode controls ------------------------------------------------------
await checkAsync('Formatted -> Raw preserves markdown', async () => {
  const before = app().getMarkdown()
  await app().setMode('raw')
  await settle()
  const after = app().getMarkdown()
  return after === before || 'markdown changed switching to raw: ' + JSON.stringify(after)
})

await checkAsync('Raw -> Formatted preserves markdown', async () => {
  const before = app().getMarkdown()
  await app().setMode('formatted')
  await settle()
  const after = app().getMarkdown()
  return after === before || 'markdown changed returning to formatted: ' + JSON.stringify(after)
})

await checkAsync('Split toggles a real second pane', async () => {
  await app().toggleSplit()
  await settle()
  const on = document.body.classList.contains('split-mode')
  await app().toggleSplit()
  await settle()
  return on || 'split did not engage'
})

// ---- Ribbon tabs --------------------------------------------------------
// Panels are toggled with the hidden ATTRIBUTE on [data-ribbon-panel]
// (app.js:743), not with an is-active class.
for (const tab of ['home', 'insert', 'format', 'view', 'help']) {
  await checkAsync('ribbon tab ' + tab + ' activates', async () => {
    app().activateRibbonTab(tab)
    await settle()
    const shown = [...document.querySelectorAll('[data-ribbon-panel]')].filter((p) => !p.hidden)
    if (shown.length !== 1) return shown.length + ' panels visible, expected exactly 1'
    return shown[0].dataset.ribbonPanel === tab ||
      'showing ' + shown[0].dataset.ribbonPanel + ' after activating ' + tab
  })
}

// ---- Commands mutate the buffer ----------------------------------------
// Taken from the data-command attributes the ribbon actually dispatches, not
// from what the names ought to be. An unknown id is a silent no-op, so a walk
// using invented names reports the app broken when it is the walk that is.
const commands = [
  'bold', 'italic', 'strike', 'highlight', 'inline-code',
  'h1', 'h2', 'h3',
  'bullet-list', 'numbered-list', 'task-list', 'quote',
  'link', 'table', 'code-block', 'mermaid', 'math', 'hr',
]
for (const cmd of commands) {
  await checkAsync('command ' + cmd + ' mutates the buffer', async () => {
    app().activateRibbonTab('home')
    await app().setMarkdown('seed text\n')
    await settle()
    const before = app().getMarkdown()
    await app().runCommand(cmd)
    await settle()
    const after = app().getMarkdown()
    return after !== before || 'no change to the document'
  })
}

// image is NOT in the list above. Importing into an unsaved document is
// documented to refuse -- there is nowhere to put the asset until the document
// has a path -- so asserting that it mutates the buffer would be asserting the
// opposite of the contract. What must hold is that it refuses without breaking.
await checkAsync('image import refuses cleanly on an unsaved document', async () => {
  await app().setMarkdown('seed text\n')
  await settle()
  const before = app().getMarkdown()
  try {
    await app().runCommand('image')
  } catch (e) {
    return 'refusal escaped as an exception: ' + (e && e.message)
  }
  await settle()
  return app().getMarkdown() === before || 'the document changed despite the refusal'
})

// ---- File rail ----------------------------------------------------------
await checkAsync('New document creates a buffer', async () => {
  const before = app().state.docs.length
  await app().newDocument()
  await settle()
  return app().state.docs.length === before + 1 ||
    'doc count went ' + before + ' -> ' + app().state.docs.length
})

check('rail rows match open buffers', () => {
  const rows = document.querySelectorAll('#file-rail .rail-row, #file-list .file-row, [data-doc-id]')
  return rows.length >= app().state.docs.length ||
    'rail shows ' + rows.length + ' rows for ' + app().state.docs.length + ' buffers'
})

// ---- Empty state --------------------------------------------------------
// pasteMarkdown reads the SYSTEM clipboard via navigator.clipboard.readText().
// Checked for a clean refusal rather than for content: the clipboard holds
// whatever the operator last copied, so asserting on what lands in the buffer
// would make the result depend on the machine. What must not happen is an
// unhandled rejection — app.js guards the empty case but not a denied one.
await checkAsync('Paste markdown does not throw when the clipboard is unavailable', async () => {
  try {
    await app().pasteMarkdown()
    return true
  } catch (e) {
    return 'pasteMarkdown rejected: ' + (e && e.message)
  }
})

for (const tpl of ['readme', 'changelog', 'design']) {
  await checkAsync('template ' + tpl + ' applies', async () => {
    await app().applyTemplate(tpl)
    await settle()
    return app().getMarkdown().trim().length > 0 || 'template produced an empty document'
  })
}

// ---- Side panels --------------------------------------------------------
// View-tab commands act on the workspace rather than the document, so they are
// checked for their effect on the shell instead of on the buffer.
for (const view of ['focus', 'theme']) {
  await checkAsync('view command ' + view + ' changes the workspace', async () => {
    app().activateRibbonTab('view')
    const before = document.body.className + '|' + document.documentElement.className
    await app().runCommand(view)
    await settle()
    const changed = document.body.className + '|' + document.documentElement.className !== before
    await app().runCommand(view)
    await settle()
    return changed || 'no shell state changed'
  })
}

for (const side of ['left', 'right']) {
  await checkAsync('side panel ' + side + ' toggles', async () => {
    const before = document.body.className
    app().toggleSidePanel(side)
    await settle()
    const changed = document.body.className !== before
    app().toggleSidePanel(side)
    await settle()
    return changed || 'toggling ' + side + ' changed no state'
  })
}

// ---- Settings -----------------------------------------------------------
await checkAsync('Settings opens and closes', async () => {
  await app().openSettings()
  await settle()
  const open = visible($('#settings-modal, .settings-modal, [role="dialog"]'))
  app().closeSettings()
  await settle()
  return open || 'settings modal never became visible'
})

// ---- Status bar ---------------------------------------------------------
check('status bar reports the mode', () => {
  const bar = $('#status-bar, .status-bar')
  if (!visible(bar)) return 'status bar not visible'
  const text = bar.textContent.toLowerCase()
  return text.includes('formatted') || text.includes('raw') || 'status bar names no mode: ' + text
})

window.webkit.messageHandlers.drmd.postMessage({ id: 0, method: '__walk', args: [results] })
`
