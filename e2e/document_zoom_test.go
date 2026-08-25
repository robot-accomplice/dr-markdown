package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// Document zoom brings the whole document closer, preserving proportions —
// the sense Word means it, not "bigger type".
//
// It is implemented with CSS `zoom` rather than `transform: scale` because zoom
// takes part in layout: the pane can be scrolled to reach content that has grown.
// A transform paints at the new size while the parent lays out the old one, so a
// zoomed-in document overflows with nothing to scroll to. That distinction is
// what this gate defends — it checks the SCROLL EXTENT grows, which a transform
// would leave untouched while still looking bigger in a screenshot.
func TestDocumentZoomScalesTheWholePaneAndItsScrollExtent(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	// Long enough to overflow the pane at 100%. With a short document the
	// scroll extent is just the pane's own height at every zoom level, so the
	// layout assertion below would compare 759 against 759 and prove nothing.
	body := strings.Repeat("Some body text in a paragraph that takes a line.\n\n", 40)
	fixture := "# Title\n\n" + body + "```go\nfunc main() {}\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#document-zoom') !== null`) {
		t.Fatal("the document pane should carry a zoom control")
	}

	read := `(async () => {
		await new Promise((r) => setTimeout(r, 250))
		const host = document.getElementById('editor-host')
		const region = document.getElementById('document-region')
		const h = host.getBoundingClientRect()
		return {
			width: Math.round(h.width),
			height: Math.round(h.height),
			scroll: region.scrollHeight,
			label: document.querySelector('[data-zoom-level]').textContent.trim(),
		}
	})()`

	var base, zoomed, reset struct {
		Width, Height, Scroll int
		Label                 string
	}
	evalJS(t, ctx, read, &base)
	if base.Label != "100%" {
		t.Errorf("zoom should start at 100%%, shows %q", base.Label)
	}

	// Three steps in.
	for i := 0; i < 3; i++ {
		clickWhenVisible(t, ctx, `#document-zoom [data-zoom="in"]`)
	}
	evalJS(t, ctx, read, &zoomed)

	if zoomed.Width <= base.Width {
		t.Errorf("zooming in did not widen the document: %d then %d", base.Width, zoomed.Width)
	}
	if zoomed.Height <= base.Height {
		t.Errorf("zooming in did not make the document taller: %d then %d", base.Height, zoomed.Height)
	}
	// The proportion check: both axes grow by the same factor, which is what
	// "preserving proportions" means and what a font-size change would not do.
	wRatio := float64(zoomed.Width) / float64(base.Width)
	hRatio := float64(zoomed.Height) / float64(base.Height)
	if diff := wRatio - hRatio; diff > 0.05 || diff < -0.05 {
		t.Errorf("zoom did not preserve proportions: width scaled %.2fx, height %.2fx", wRatio, hRatio)
	}
	// The layout check: a transform would leave this untouched.
	if zoomed.Scroll <= base.Scroll {
		t.Errorf("the pane's scroll extent did not grow with the zoom (%d then %d): "+
			"the document is painted larger but cannot be scrolled to, which is what "+
			"using transform instead of zoom would produce", base.Scroll, zoomed.Scroll)
	}
	if zoomed.Label != "130%" {
		t.Errorf("three steps from 100%% should read 130%%, reads %q", zoomed.Label)
	}

	// The level is a reset button.
	clickWhenVisible(t, ctx, `#document-zoom [data-zoom="reset"]`)
	evalJS(t, ctx, read, &reset)
	if reset.Label != "100%" || reset.Width != base.Width {
		t.Errorf("clicking the level should reset to 100%% and the original width; got %q at %dpx",
			reset.Label, reset.Width)
	}
}

// The control must not zoom itself: inside the zoomed host it would shrink as
// you zoomed out, becoming smallest exactly when it is hardest to hit.
func TestZoomControlDoesNotZoomItself(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Title\\n').then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#document-zoom') !== null`) {
		t.Fatal("no zoom control")
	}

	var inside bool
	evalJS(t, ctx, `document.getElementById('editor-host').contains(document.getElementById('document-zoom'))`, &inside)
	if inside {
		t.Error("the zoom control is inside the zoomed host, so it scales with the document")
	}

	var before, after int
	evalJS(t, ctx, `Math.round(document.getElementById('document-zoom').getBoundingClientRect().height)`, &before)
	for i := 0; i < 3; i++ {
		clickWhenVisible(t, ctx, `#document-zoom [data-zoom="out"]`)
	}
	evalJS(t, ctx, `Math.round(document.getElementById('document-zoom').getBoundingClientRect().height)`, &after)
	if before != after {
		t.Errorf("the zoom control resized with the document: %dpx then %dpx", before, after)
	}
}
