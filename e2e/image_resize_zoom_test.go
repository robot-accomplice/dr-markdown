package e2e

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// Dragging an image's resize handle by N pixels grows the image by N pixels,
// at any zoom.
//
// Reported from real use at 120%: the banner came back from ordinary clicking
// around the document clipped on both sides, the mark half cut off. The Crepe
// resize handler measures the drag in VIEWPORT pixels — clientY minus
// getBoundingClientRect().top, both zoom-scaled — and writes the result as a
// style height INSIDE the zoomed context, which scales it a second time. One
// 5px drag at 120% set the height 35px larger (measured on the real host with
// DRMD_PROBE_IMG=1: 149.96px became 184.93px, and 184.93 = 149.96 x 1.2 + 5).
// object-fit: cover then scales the content up to the too-tall box, cropping
// the sides — the clipped banner.
//
// The handle is 4px tall, invisible until hover, and parked on the image's
// bottom edge, so the drag needs no intent: a click that lands a pixel low and
// moves is a resize.
//
// Asserted as the invariant rather than the arithmetic: an 8px downward drag
// grows the RENDERED height by 8px, whatever the zoom.
//
// The handle is display:none in the app (accidental drags cropped images
// sideways even with correct math, and the result never survived a mode
// switch). The test force-shows it: the vendored drag arithmetic is still
// live code, still wrong without the patch, and still worth a gate if the
// handle is ever re-enabled.
func TestImageResizeHandleDragsInViewportPixelsUnderZoom(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{"# Wide", "",
		"![banner](data:image/png;base64," + wideImageB64 + ")", "", "text", ""}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	var m struct {
		Before float64 `json:"before"`
		After  float64 `json:"after"`
		Drag   float64 `json:"drag"`
	}
	evalJS(t, ctx, `(async () => {
		const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
		const img0 = document.querySelector('#wysiwyg img')
		for (let i = 0; i < 100 && !(img0 && img0.complete && img0.naturalWidth > 0 && img0.style.height); i++) {
			await sleep(100)
		}
		document.documentElement.style.setProperty('--doc-zoom', '1.2')
		await sleep(700)
		const img = document.querySelector('#wysiwyg img')
		const handle = document.querySelector('#wysiwyg .image-resize-handle')
		if (!img || !handle) return { before: -1, after: -1, drag: -1 }
		// The app hides the handle; this test measures the handler's arithmetic,
		// so show it for the duration.
		handle.style.display = 'block'
		const before = img.getBoundingClientRect().height
		const hr = handle.getBoundingClientRect()
		const cx = hr.x + hr.width / 2
		const cy = hr.y + hr.height / 2
		const drag = 8
		const opts = (y) => ({ bubbles: true, pointerId: 1, clientX: cx, clientY: y, button: 0, buttons: 1 })
		handle.dispatchEvent(new PointerEvent('pointerdown', opts(cy)))
		window.dispatchEvent(new PointerEvent('pointermove', opts(cy + drag)))
		window.dispatchEvent(new PointerEvent('pointerup', opts(cy + drag)))
		await sleep(300)
		return { before, after: img.getBoundingClientRect().height, drag }
	})()`, &m)

	if m.Before <= 0 {
		t.Fatalf("image or resize handle never rendered: %+v", m)
	}
	got := m.After - m.Before
	if math.Abs(got-m.Drag) > 2 {
		t.Errorf("an %.0fpx drag at 120%% zoom changed the rendered height by %.1fpx, not %.0fpx",
			m.Drag, got, m.Drag)
	}
}
