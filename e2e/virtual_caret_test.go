package e2e

import (
	"strconv"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// The caret a user sees in the formatted editor is not the native one: Crepe
// runs ProseMirror's virtual-cursor plugin, which suppresses the native caret
// and draws its own .prosemirror-virtual-cursor div INSIDE the zoomed
// container. The plugin positions that div in viewport-convention
// coordinates, and in WKWebView the container's CSS zoom then scales them a
// SECOND time: at 130% the painted caret lands 0.3x away from the zoom
// origin — below, to the right, and 1.3x too tall — while typed text still
// lands exactly where the user clicked. Measured in the real host: selection
// rect (433.1, 268.2, h 23.0), plugin div painted at (461.8, 284.2, h 29.9).
//
// The app's answer at zoom != 100% is to hide the plugin's div and draw its
// own caret — #virtual-caret, a fixed-position element OUTSIDE the zoom
// context, placed at the collapsed selection's own client rect, which is the
// one coordinate system proven to paint truthfully in WKWebView.
//
// Chrome keeps the plugin's math consistent, so it never shows the
// displacement — but the overlay LOGIC is engine-independent, and that is
// what this gate holds. The real-host verdict belongs to the -cursor-probe
// harness, which drives WKWebView itself.
func TestVirtualCaretTracksTheSelectionWhenZoomed(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "Alpha bravo charlie delta echo foxtrot.\n\nSecond paragraph with several words here.\n\nThird paragraph to finish things off.\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelectorAll('#wysiwyg .ProseMirror p').length >= 3`) {
		t.Fatal("wysiwyg did not render three paragraphs")
	}

	placeCaret := `(() => {
		const p = document.querySelectorAll('#wysiwyg .ProseMirror p')[1]
		const range = document.createRange()
		range.setStart(p.firstChild, 10)
		range.collapse(true)
		const sel = window.getSelection()
		sel.removeAllRanges()
		sel.addRange(range)
		return true
	})()`

	readCaret := `(() => {
		const vc = document.getElementById('virtual-caret')
		const plugin = document.querySelector('.prosemirror-virtual-cursor')
		const sel = window.getSelection()
		const rects = sel.rangeCount ? sel.getRangeAt(0).getClientRects() : []
		const r = rects.length ? rects[0] : null
		return {
			exists: vc !== null,
			hidden: vc ? vc.hidden : true,
			x: vc ? vc.getBoundingClientRect().x : -1,
			y: vc ? vc.getBoundingClientRect().y : -1,
			h: vc ? vc.getBoundingClientRect().height : -1,
			selX: r ? r.x : -1,
			selY: r ? r.y : -1,
			selH: r ? r.height : -1,
			pluginVisible: plugin ? getComputedStyle(plugin).display !== 'none' : false,
		}
	})()`

	// caretRead mirrors the readCaret payload above.
	type caretRead struct {
		Exists           bool
		Hidden           bool
		X, Y, H          float64
		SelX, SelY, SelH float64
		PluginVisible    bool
	}

	var placed bool
	evalJS(t, ctx, placeCaret, &placed)
	var at100 caretRead
	evalJS(t, ctx, readCaret, &at100)
	if at100.Exists && !at100.Hidden {
		t.Error("at 100% zoom the virtual caret should be hidden — the plugin's own cursor is truthful there")
	}

	for i := 0; i < 3; i++ {
		clickWhenVisible(t, ctx, `#document-zoom [data-zoom="in"]`)
	}
	evalJS(t, ctx, placeCaret, &placed)
	if !waitForJS(t, ctx, `(() => { const vc = document.getElementById('virtual-caret'); return vc && !vc.hidden })()`) {
		t.Fatal("at 130% zoom the virtual caret should be visible over a collapsed editor selection")
	}

	var zoomed caretRead
	evalJS(t, ctx, readCaret, &zoomed)
	if zoomed.PluginVisible {
		t.Error("at 130% zoom the plugin's own cursor div should be hidden — it is the element that paints displaced")
	}
	if zoomed.SelX < 0 {
		t.Fatal("the collapsed selection reported no client rect")
	}
	if diff := zoomed.X - zoomed.SelX; diff > 1 || diff < -1 {
		t.Errorf("virtual caret x %.1f is not the selection rect x %.1f", zoomed.X, zoomed.SelX)
	}
	if diff := zoomed.Y - zoomed.SelY; diff > 1 || diff < -1 {
		t.Errorf("virtual caret y %.1f is not the selection rect y %.1f", zoomed.Y, zoomed.SelY)
	}
	if diff := zoomed.H - zoomed.SelH; diff > 1 || diff < -1 {
		t.Errorf("virtual caret height %.1f is not the selection rect height %.1f", zoomed.H, zoomed.SelH)
	}

	// Typing moves the insertion point; the virtual caret must follow.
	// selectionchange is dispatched asynchronously, so poll rather than read
	// the very next moment.
	for _, ch := range []string{"X", "Y", "Z"} {
		if err := chromedp.Run(ctx, chromedp.KeyEvent(ch)); err != nil {
			t.Fatalf("type %s: %v", ch, err)
		}
	}
	if !waitForJS(t, ctx, `(() => {
		const vc = document.getElementById('virtual-caret')
		const sel = window.getSelection()
		if (!vc || vc.hidden || !sel.rangeCount) return false
		const rects = sel.getRangeAt(0).getClientRects()
		if (!rects.length) return false
		return Math.abs(vc.getBoundingClientRect().x - rects[0].x) <= 1
	})()`) {
		t.Fatal("the virtual caret never caught up with the selection after typing")
	}
	var afterType caretRead
	evalJS(t, ctx, readCaret, &afterType)
	if afterType.Hidden {
		t.Error("the virtual caret vanished after typing")
	}
	if diff := afterType.X - afterType.SelX; diff > 1 || diff < -1 {
		t.Errorf("after typing, virtual caret x %.1f is not the selection rect x %.1f", afterType.X, afterType.SelX)
	}
	if afterType.X <= zoomed.X {
		t.Errorf("typing did not advance the virtual caret: x %.1f then %.1f", zoomed.X, afterType.X)
	}

	var md string
	evalJS(t, ctx, "window.__app.getEditorMarkdown()", &md)
	if !strings.Contains(md, "Second parXYZagraph") {
		t.Errorf("the overlay must not change where text lands; markdown:\n%s", md)
	}

	// Back at 100% the plugin's own cursor is restored and the overlay retires.
	clickWhenVisible(t, ctx, `#document-zoom [data-zoom="reset"]`)
	var reset caretRead
	evalJS(t, ctx, readCaret, &reset)
	if reset.Exists && !reset.Hidden {
		t.Error("the virtual caret should retire when the zoom returns to 100%")
	}
}
