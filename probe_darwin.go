//go:build darwin

package main

/*
#include <stdlib.h>
void hostEvalJS(const char *js);
void hostProbeClick(double x, double y);
void hostProbeType(const char *chars);
void hostProbeExternalPoint(double x, double y, double *outX, double *outY);
void hostProbeFocus(void);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unsafe"
)

// TEMPORARY DIAGNOSTIC — not a gate, not shipped behaviour.
//
// A probe for the reported WYSIWYG caret bug: the caret lands where the user
// clicks, but typed text appears elsewhere. The Chrome/chromedp probe
// (e2e/cursor_probe_test.go) does NOT reproduce it, which points at the real
// host: WKWebView's caret coordinate mapping, with CSS `zoom` (the document
// zoom feature) as the leading suspect.
//
// The flow, mirrored from the Chrome probe but with REAL AppKit events:
//
//  1. This script loads the fixture, normalizes the zoom through the real
//     zoom controls (boot zoom comes from PERSISTED preferences, so without
//     normalization "130%" would silently be whatever the last session left
//     plus three steps), computes the click target's viewport coordinates,
//     and posts phase "target".
//  2. The native side synthesizes a real left click at those coordinates and
//     posts it through the AppKit event queue — a JS-dispatched click would
//     not exercise WebKit's coordinate mapping, which is the thing under test.
//  3. On selectionchange the probe posts phase "selection": where the caret
//     VISIBLY is.
//  4. The native side synthesizes key events for X, Y, Z.
//  5. 400ms after the last input event (or 3s after the click if no input
//     arrives — a MISMATCH is a finding, a hang is not) the probe posts phase
//     "result" with the editor's markdown.
//
// The Go side prints every phase and judges: "Second parXYZagraph" present
// means typed text landed where the click put the caret.
const probeModuleJS = `
const post = (payload) =>
  window.webkit.messageHandlers.drmd.postMessage({ id: 0, method: '__probe', args: [payload] })
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

for (let i = 0; i < 200 && !globalThis.__app?.ready; i++) {
  await sleep(50)
}
if (!globalThis.__app?.ready) {
  post({ phase: 'error', message: 'app never became ready' })
  throw new Error('app never became ready')
}

// The bug report is about the WYSIWYG surface; #wysiwyg/.ProseMirror exists
// only there. 'wysiwyg' is the mode name setMode() actually switches on.
await globalThis.__app.setMode('wysiwyg')

const fixture = "Alpha bravo charlie delta echo foxtrot.\n\nSecond paragraph with several words here.\n\nThird paragraph to finish things off.\n"
await globalThis.__app.setMarkdown(fixture)

let ps = []
for (let i = 0; i < 100; i++) {
  ps = document.querySelectorAll('#wysiwyg .ProseMirror p')
  if (ps.length >= 3) break
  await sleep(100)
}
if (ps.length < 3) {
  post({ phase: 'error', message: 'expected 3+ paragraphs, found ' + ps.length })
  throw new Error('paragraphs not rendered')
}

const zoomLabel = () => {
  const el = document.querySelector('[data-zoom-level]')
  return el ? el.textContent : '?'
}
const zoomAtBoot = zoomLabel()
const resetBtn = document.querySelector('[data-zoom="reset"]')
if (resetBtn) resetBtn.click()
const requested = globalThis.__drmdProbeZoom || '1.0'
if (requested === '1.3') {
  const inBtn = document.querySelector('[data-zoom="in"]')
  for (let i = 0; i < 3; i++) { if (inBtn) inBtn.click() }
}
if (requested === '0.9') {
  const outBtn = document.querySelector('[data-zoom="out"]')
  if (outBtn) outBtn.click()
}
await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))

ps = document.querySelectorAll('#wysiwyg .ProseMirror p')
if (ps.length < 3) {
  post({ phase: 'error', message: 'paragraphs lost after zoom: ' + ps.length })
  throw new Error('paragraphs lost after zoom')
}
const p = ps[1]
const text = p.firstChild
const range = document.createRange()
range.setStart(text, 10)
range.setEnd(text, 10)
const rect = range.getBoundingClientRect()
const clickX = rect.x + 1
const clickY = rect.y + rect.height / 2

let resultPosted = false
let resultTimer = null
const postResult = (why) => {
  if (resultPosted) return
  resultPosted = true
  clearTimeout(resultTimer)
  const sel = window.getSelection()
  const psNow = document.querySelectorAll('#wysiwyg .ProseMirror p')
  let anchorPara = -1
  for (let i = 0; i < psNow.length; i++) {
    if (psNow[i].contains(sel.anchorNode)) anchorPara = i
  }
  post({
    phase: 'result',
    why: why,
    markdown: globalThis.__app.getEditorMarkdown(),
    para: anchorPara,
    offset: sel.anchorOffset,
  })
}
document.addEventListener('input', () => {
  clearTimeout(resultTimer)
  resultTimer = setTimeout(() => postResult('400ms after the last input event'), 400)
}, true)

// EVERY selection change is reported, not just the first. A human who sees the
// caret land wrong clicks again — and a probe that only reports the first
// change silently re-aims the run at the second click.
let selSeq = 0
document.addEventListener('selectionchange', () => {
  const sel = window.getSelection()
  let anchorPara = -1
  for (let i = 0; i < ps.length; i++) {
    if (ps[i].contains(sel.anchorNode)) anchorPara = i
  }
  // The caret's own rect as the DOM computes it. If the PAINTED caret and
  // this rect disagree, the defect is in painting; if this rect disagrees
  // with the click point, the defect is in hit-testing. A collapsed range
  // reports one zero-width rect at the caret.
  let caret = null
  if (sel.rangeCount > 0) {
    const rects = sel.getRangeAt(0).getClientRects()
    if (rects.length > 0) caret = { x: rects[0].x, y: rects[0].y, h: rects[0].height }
  }
  post({
    phase: 'selection',
    seq: selSeq++,
    para: anchorPara,
    offset: sel.anchorOffset,
    text: sel.anchorNode ? String(sel.anchorNode.textContent) : '',
    caretX: caret ? caret.x : -1,
    caretY: caret ? caret.y : -1,
    caretH: caret ? caret.h : -1,
    // Crepe runs ProseMirror's virtual-cursor plugin: caret-color is
    // transparent on .ProseMirror and the visible caret is the plugin's own
    // .prosemirror-virtual-cursor div — displaced under WKWebView zoom, which
    // is the whole defect. The fix hides that div at zoom != 100% and draws
    // #virtual-caret instead. Report BOTH rects, so the run shows which
    // element is where: before the fix the plugin div is displaced; after it,
    // the plugin div is display:none (zero rect) and the overlay sits exactly
    // on the DOM selection rect.
    vc: (() => {
      const read = (el) => {
        if (!el) return null
        const r = el.getBoundingClientRect()
        return { x: r.x, y: r.y, h: r.height, display: getComputedStyle(el).display }
      }
      return {
        plugin: read(document.querySelector('.prosemirror-virtual-cursor')),
        overlay: read(document.getElementById('virtual-caret')),
      }
    })(),
  })
  // External mode waits on a HUMAN, who has to read the instruction, find the
  // window, and type. 3s was tuned for synthesized input and produced a false
  // MISMATCH before the person had finished reading. Re-armed per change: the
  // budget is 20s after the LAST selection movement.
  const settleMs = globalThis.__drmdProbeExternal ? 20000 : 3000
  clearTimeout(resultTimer)
  resultTimer = setTimeout(() => postResult(settleMs + 'ms after the last selection with no settled input'), settleMs)
})

post({
  phase: 'target',
  x: clickX,
  y: clickY,
  zoomAtBoot: zoomAtBoot,
  zoomNow: zoomLabel(),
  requested: requested,
  dpr: window.devicePixelRatio,
  viewportW: window.innerWidth,
  viewportH: window.innerHeight,
  paraCount: ps.length,
  paraText: String(p.textContent),
})
`

// bareProbeModuleJS isolates the engine from the editor: a bare
// contenteditable with nothing on it but CSS `zoom`, overlaid on the app. No
// Crepe, no ProseMirror, no app CSS in the zoomed subtree. The selection is
// set PROGRAMMATICALLY — hit-testing is not under test here, only caret
// painting. If the painted caret is still displaced, the defect is WebKit's
// and every editor on this host inherits it; if it paints true, the
// interaction is specific to the editor stack.
const bareProbeModuleJS = `
const post = (payload) =>
  window.webkit.messageHandlers.drmd.postMessage({ id: 0, method: '__probe', args: [payload] })
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

for (let i = 0; i < 200 && !globalThis.__app?.ready; i++) {
  await sleep(50)
}
if (!globalThis.__app?.ready) {
  post({ phase: 'error', message: 'app never became ready' })
  throw new Error('app never became ready')
}

const zoom = globalThis.__drmdProbeZoom || '1.3'
// Fixed spacer keeps the rig on screen over the app; the zoomed host is an
// IN-FLOW child of it, mirroring the app where zoom is on the in-flow
// #editor-host ancestor of the editable, not on the editable itself.
const spacer = document.createElement('div')
spacer.style.cssText = 'position:fixed;left:150px;top:200px;z-index:2147483646;'
const host = document.createElement('div')
host.id = 'bareprobe'
host.style.cssText = 'background:#fff;border:1px solid #888;width:640px;zoom:' + zoom + ';'
const ed = document.createElement('div')
ed.contentEditable = 'true'
ed.style.cssText = 'font:16px/1.5 -apple-system,BlinkMacSystemFont,sans-serif;padding:24px;outline:none;'
ed.textContent = 'Alpha bravo charlie delta echo foxtrot.'
host.appendChild(ed)
spacer.appendChild(host)
document.body.appendChild(spacer)
ed.focus()

const text = ed.firstChild
const range = document.createRange()
range.setStart(text, 10)
range.collapse(true)
const rect = range.getBoundingClientRect()
const sel = window.getSelection()
sel.removeAllRanges()
sel.addRange(range)

// Ground truth, drawn by the SAME engine that lays out the text: a 2px red
// bar at exactly the DOM caret rect of the CURRENT selection, OUTSIDE the
// zoom context so no zoom applies to it. If WebKit paints the caret on the
// red bar, painting is correct; any daylight between them is the defect,
// measured in the photograph rather than by eye.
let marker = null
let selSeq = 0
document.addEventListener('selectionchange', () => {
  const s = window.getSelection()
  let caret = null
  if (s.rangeCount > 0) {
    const rects = s.getRangeAt(0).getClientRects()
    if (rects.length > 0) caret = { x: rects[0].x, y: rects[0].y, h: rects[0].height }
  }
  if (caret) {
    if (!marker) {
      marker = document.createElement('div')
      document.body.appendChild(marker)
    }
    // 6px to the RIGHT of the caret rect: near enough to compare at a glance,
    // far enough never to occlude the painted caret underneath.
    marker.style.cssText = 'position:fixed;left:' + (caret.x + 6) + 'px;top:' + caret.y + 'px;width:2px;height:' +
      caret.h + 'px;background:red;z-index:2147483647;pointer-events:none;'
  }
  post({
    phase: 'selection',
    seq: selSeq++,
    para: ed.contains(s.anchorNode) ? 1 : -1,
    offset: s.anchorOffset,
    text: s.anchorNode ? String(s.anchorNode.textContent) : '',
    caretX: caret ? caret.x : -1,
    caretY: caret ? caret.y : -1,
    caretH: caret ? caret.h : -1,
  })
})

post({
  phase: 'target',
  x: rect.x,
  y: rect.y + rect.height / 2,
  zoomAtBoot: 'bare',
  zoomNow: zoom,
  requested: zoom,
  dpr: window.devicePixelRatio,
  viewportW: window.innerWidth,
  viewportH: window.innerHeight,
  paraCount: 1,
  paraText: 'bare contenteditable, caret set programmatically at offset 10',
})

setTimeout(() => {
  post({ phase: 'result', why: 'bare probe observation window elapsed', markdown: ed.textContent, para: 1, offset: sel.anchorOffset })
}, 4000)
`

// probeMessage is one phase report from the injected probe script. Every field
// is optional in the payload — which phase populates which is visible in
// probeModuleJS above.
type probeMessage struct {
	Phase     string  `json:"phase"`
	Message   string  `json:"message"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Para      int     `json:"para"`
	Offset    int     `json:"offset"`
	Text      string  `json:"text"`
	Markdown  string  `json:"markdown"`
	Why       string  `json:"why"`
	ZoomBoot  string  `json:"zoomAtBoot"`
	ZoomNow   string  `json:"zoomNow"`
	Requested string  `json:"requested"`
	DPR       float64 `json:"dpr"`
	ViewW     int     `json:"viewportW"`
	ViewH     int     `json:"viewportH"`
	ParaCount int     `json:"paraCount"`
	ParaText  string  `json:"paraText"`
	CaretX    float64 `json:"caretX"`
	CaretY    float64 `json:"caretY"`
	CaretH    float64 `json:"caretH"`
	Seq       int     `json:"seq"`
	VC        *struct {
		Plugin  *probeRect `json:"plugin"`
		Overlay *probeRect `json:"overlay"`
	} `json:"vc"`
}

// probeRect is one measured element rectangle from the probe script.
type probeRect struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	H       float64 `json:"h"`
	Display string  `json:"display"`
}

// The last target the probe asked for, in GLOBAL screen coordinates (Quartz,
// top-left origin). Saved so the selection phase can photograph the painted
// caret where the click actually landed.
var probeGlobalX, probeGlobalY float64

// probeBare (DRMD_PROBE_BARE=1) swaps the Crepe fixture for a bare
// contenteditable with nothing but CSS zoom on it, and places the caret
// programmatically. It isolates caret PAINTING from the editor stack.
var probeBare = os.Getenv("DRMD_PROBE_BARE") == "1"

// probeScreenshots photographs the screen around the last probe target. The
// DOM selection is only half the story — the DEFECT report is about where the
// caret is PAINTED. Shot 0 keeps the MOUSE CURSOR in frame (-C): with a human
// clicking, it is the only exact record of where the click actually went.
// Three shots across a blink cycle so one of them catches the caret lit.
//
// Full-screen, not -R regions: region capture started failing mid-session
// ("could not create image from rect") while full-screen capture kept
// working. The reviewer crops to the target with the image tooling instead.
func probeScreenshots() {
	for i, delay := range []time.Duration{0, 300 * time.Millisecond, 600 * time.Millisecond} {
		time.Sleep(delay)
		out := fmt.Sprintf("/tmp/probe-caret-%d.png", i)
		args := []string{"-x", out}
		if i == 0 {
			args = []string{"-x", "-C", out}
		}
		if err := exec.Command("screencapture", args...).Run(); err != nil {
			fmt.Printf("PROBE screenshot %d failed: %v\n", i, err)
		} else {
			fmt.Printf("PROBE screenshot: %s (target at global %.0f,%.0f)\n", out, probeGlobalX, probeGlobalY)
		}
	}
}

// reportProbe prints each phase as it arrives and drives the next one. The
// target phase is answered with a REAL click, the selection phase with REAL
// key events — both fabricated in Objective-C and posted through the AppKit
// event queue, because the defect under investigation is in the coordinate
// mapping real events take.
func reportProbe(argsJSON string) {
	var payload []probeMessage
	if err := json.Unmarshal([]byte(argsJSON), &payload); err != nil || len(payload) == 0 {
		fmt.Printf("PROBE: unreadable payload %q: %v\n", argsJSON, err)
		fmt.Println("VERDICT: FAIL (unreadable report)")
		os.Exit(1)
	}
	msg := payload[0]

	switch msg.Phase {
	case "target":
		fmt.Printf("PROBE target: x=%.1f y=%.1f viewport=%dx%d dpr=%.2f\n",
			msg.X, msg.Y, msg.ViewW, msg.ViewH, msg.DPR)
		fmt.Printf("PROBE zoom: requested=%s atBoot=%s now=%s\n",
			msg.Requested, msg.ZoomBoot, msg.ZoomNow)
		fmt.Printf("PROBE document: %d paragraphs, paragraph[1]=%q\n", msg.ParaCount, msg.ParaText)
		if probeBare {
			// The caret was placed programmatically, but an unfocused webview
			// paints no caret. A POSTED click focuses the view for real — and
			// posted-event hit-testing is proven exact, so it lands on the
			// target. Photograph on a timer, not on selectionchange: the click
			// can land on the exact offset already selected, and an unchanged
			// selection fires no event.
			var gx, gy C.double
			C.hostProbeExternalPoint(C.double(msg.X), C.double(msg.Y), &gx, &gy)
			probeGlobalX, probeGlobalY = float64(gx), float64(gy)
			fmt.Printf("PROBE bare: DOM caret mid at GLOBAL screen x=%.1f y=%.1f — activating, then posting a click to focus\n", gx, gy)
			C.hostProbeFocus()
			time.Sleep(300 * time.Millisecond)
			C.hostProbeClick(C.double(msg.X), C.double(msg.Y))
			time.Sleep(400 * time.Millisecond)
			// One typed character: proves focus (the letter lands or it does
			// not) and forces the caret solid for the photographs.
			q, freeQ := cstr("Q")
			C.hostProbeType(q)
			freeQ()
			time.Sleep(800 * time.Millisecond)
			probeScreenshots()
			break
		}
		if cursorProbeExternal {
			// A posted event is exactly what "external" exists to rule out.
			// Report the GLOBAL screen point and let the human (or System
			// Events) drive the click and the typing; the probe script reports
			// selection and result on its own as the input lands.
			var gx, gy C.double
			C.hostProbeExternalPoint(C.double(msg.X), C.double(msg.Y), &gx, &gy)
			probeGlobalX, probeGlobalY = float64(gx), float64(gy)
			fmt.Printf("PROBE external: click at GLOBAL screen x=%.1f y=%.1f, then type XYZ\n", gx, gy)
			break
		}
		// Focus first: a window that is not key paints no caret, and the
		// photographs are the point of this probe.
		C.hostProbeFocus()
		time.Sleep(300 * time.Millisecond)
		C.hostProbeClick(C.double(msg.X), C.double(msg.Y))

	case "selection":
		fmt.Printf("PROBE selection #%d: paragraph=%d offset=%d text=%q\n",
			msg.Seq, msg.Para, msg.Offset, msg.Text)
		fmt.Printf("PROBE caret rect (viewport CSS px): x=%.1f y=%.1f h=%.1f\n",
			msg.CaretX, msg.CaretY, msg.CaretH)
		if msg.VC != nil {
			if msg.VC.Plugin != nil {
				fmt.Printf("PROBE plugin cursor div: x=%.1f y=%.1f h=%.1f display=%s\n",
					msg.VC.Plugin.X, msg.VC.Plugin.Y, msg.VC.Plugin.H, msg.VC.Plugin.Display)
			} else {
				fmt.Println("PROBE no .prosemirror-virtual-cursor element in the DOM")
			}
			if msg.VC.Overlay != nil {
				fmt.Printf("PROBE #virtual-caret overlay: x=%.1f y=%.1f h=%.1f display=%s\n",
					msg.VC.Overlay.X, msg.VC.Overlay.Y, msg.VC.Overlay.H, msg.VC.Overlay.Display)
			} else {
				fmt.Println("PROBE no #virtual-caret element in the DOM")
			}
		}
		if msg.Seq > 0 {
			// The human clicked again, or typing moved the caret. Reported for
			// the record; only the FIRST selection gets photographed and
			// answered with keystrokes.
			break
		}
		if probeBare {
			// Bare mode photographs from the target phase, on a timer.
			break
		}
		probeScreenshots()
		if cursorProbeExternal {
			break
		}
		if msg.Para != 1 {
			// The coordinate conversion is wrong, not the app: the click was
			// aimed at paragraph 1. Continue anyway — where the keys land is
			// still evidence — but say the probe itself missed.
			fmt.Println("PROBE WARNING: the click landed outside paragraph 1 — " +
				"the viewport->window conversion is suspect, not (yet) the app")
		}
		chars := C.CString("XYZ")
		defer C.free(unsafe.Pointer(chars))
		C.hostProbeType(chars)

	case "result":
		fmt.Printf("PROBE result (%s): final selection paragraph=%d offset=%d\n",
			msg.Why, msg.Para, msg.Offset)
		fmt.Printf("PROBE markdown after typing:\n%s\n", msg.Markdown)
		if probeBare {
			// Nothing is typed in bare mode; the verdict is in the photographs.
			// Compare the painted caret in /tmp/probe-caret-*.png against the
			// DOM caret rect printed with the selection phase.
			fmt.Println("VERDICT: OBSERVE — is the painted caret in the screenshots " +
				"on the text line at the DOM caret rect, or displaced from it?")
			os.Exit(0)
		}
		idx := strings.Index(msg.Markdown, "XYZ")
		if idx >= 0 {
			lo := idx - 24
			if lo < 0 {
				lo = 0
			}
			hi := idx + 27
			if hi > len(msg.Markdown) {
				hi = len(msg.Markdown)
			}
			fmt.Printf("PROBE marker at byte %d: ...%q...\n", idx, msg.Markdown[lo:hi])
		} else {
			fmt.Println("PROBE marker 'XYZ' does not appear in the editor markdown at all")
		}

		// The probe drove the zoom through the REAL controls, and zoom persists
		// in preferences. Leave the machine the way a user would want it: back
		// at 100%, with a beat for the preference save to land before exit.
		reset, freeReset := cstr(`document.querySelector('[data-zoom="reset"]')?.click()`)
		C.hostEvalJS(reset)
		freeReset()
		time.Sleep(1200 * time.Millisecond)

		if cursorProbeExternal {
			// A human drives external mode, and a human explores — clicking
			// other paragraphs, typing lowercase. The "Second parXYZagraph"
			// expectation belongs to the automated flow; here the evidence is
			// the per-selection report and the photographs, so report rather
			// than judge.
			fmt.Println("VERDICT: OBSERVE — compare the overlay rect with the selection rect " +
				"per selection above, and the painted caret with the click point in the screenshots")
			os.Exit(0)
		}
		if strings.Contains(msg.Markdown, "Second parXYZagraph") {
			fmt.Println("VERDICT: PASS — marker landed where clicked")
			os.Exit(0)
		}
		fmt.Printf("VERDICT: MISMATCH — typed text landed away from the caret (marker at byte %d)\n", idx)
		os.Exit(1)

	case "error":
		fmt.Printf("PROBE error: %s\n", msg.Message)
		fmt.Println("VERDICT: FAIL (probe script error)")
		os.Exit(1)

	default:
		fmt.Printf("PROBE: unknown phase %q, raw: %s\n", msg.Phase, argsJSON)
	}
}
