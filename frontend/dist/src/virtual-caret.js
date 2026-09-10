// The virtual caret.
//
// The caret a user sees in the formatted editor is not the native one: Crepe
// runs ProseMirror's virtual-cursor plugin, which suppresses the native caret
// (caret-color: transparent on .ProseMirror) and draws its own
// .prosemirror-virtual-cursor div INSIDE the zoomed container. The plugin
// positions that div in viewport-convention coordinates, and in WKWebView the
// container's CSS zoom then scales them a SECOND time. Measured in the real
// host at 130%: DOM selection rect (433.1, 268.2, h 23.0), the plugin's div
// painted at (461.8, 284.2, h 29.9) — 0.3x the distance from the zoom origin
// to the right and below, and 1.3x too tall — while typed text still lands
// exactly where the user clicked. Chrome keeps the plugin's math consistent,
// which is why no browser test ever saw it.
//
// So at any document zoom other than 100% the plugin's div is hidden and the
// caret is drawn HERE: a fixed-position element on <body>, OUTSIDE the zoom
// context for the same reason as the zoom control (inside, it would zoom
// itself), placed at the collapsed selection's own client rect. That is the
// one mechanism proven to paint truthfully in WKWebView: a fixed element at
// getClientRects() coordinates lands exactly where the text is.
//
// At 100% the module does nothing at all — the plugin's own cursor is
// truthful there, and two carets is worse than one.
export function initVirtualCaret({ getZoom, editorRoot }) {
  const caret = document.createElement('div')
  caret.id = 'virtual-caret'
  caret.hidden = true
  document.body.appendChild(caret)

  const zoomed = () => Math.abs((getZoom() ?? 1) - 1) > 1e-9

  // The suppression attribute gates the CSS that hides the plugin's own
  // cursor div. It tracks the ZOOM, not the selection: the displaced div
  // must never flash, even between a zoom change and the next selection
  // event.
  function syncZoomState() {
    document.documentElement.dataset.virtualCaret = zoomed() ? 'on' : 'off'
  }

  function update() {
    if (!zoomed()) {
      caret.hidden = true
      return
    }
    const sel = window.getSelection()
    const anchor = sel && sel.rangeCount > 0 ? sel.anchorNode : null
    if (!sel || !sel.isCollapsed || !anchor ||
      !editorRoot.contains(anchor) || editorRoot.hidden) {
      caret.hidden = true
      return
    }
    const rects = sel.getRangeAt(0).getClientRects()
    if (rects.length === 0) {
      caret.hidden = true
      return
    }
    const rect = rects[0]
    caret.style.left = `${rect.x}px`
    caret.style.top = `${rect.y}px`
    caret.style.height = `${rect.height}px`
    caret.hidden = false
    // The blink restarts on every move: a caret that keeps blinking while you
    // type reads as lag, and a solid one settles into blinking on its own.
    caret.style.animation = 'none'
    void caret.offsetWidth
    caret.style.animation = ''
  }

  function hide() {
    caret.hidden = true
  }

  // The document scrolls inside #document-region, and a scrolling container
  // fires no selectionchange. Capture phase catches every scroller without
  // naming them.
  document.addEventListener('scroll', () => {
    if (!caret.hidden) update()
  }, true)
  window.addEventListener('resize', () => {
    if (!caret.hidden) update()
  })

  syncZoomState()
  return { update, hide, syncZoomState }
}
