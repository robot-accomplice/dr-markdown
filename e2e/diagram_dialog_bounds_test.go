package e2e

import "testing"

// The diagram assistant's preview stays inside its own box, and the fields stay
// inside theirs.
//
// Reported from real use, against the packaged 0.6.0 build: "the example diagram
// is not properly bounded and is overlapping the fields to the left". Two
// separate defects, both measured:
//
//	fields   l=428 r=668 w=240    the column, correctly capped
//	input    l=428 r=688 w=260    20px WIDER than the column it is in
//	preview  l=684                so the input ran underneath it
//
//	preview  t=386 b=732          the box
//	svg      t=377 b=741          9px out of the top AND 9px out of the bottom
//
// The input had width:100% plus 9px padding and a 1px border on a content-box
// element — 240 + 18 + 2 = 260. The svg was bounded in width but not height, and
// `place-items: center` on an item taller than its box overflows BOTH ways, so
// the top of the diagram was clipped and could not be scrolled to.
func TestTheDiagramAssistantStaysInsideItsBoxes(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	type box struct{ L, R, T, B float64 }
	var m struct {
		Fields, Input, Preview, SVG box
	}
	evalJS(t, ctx, `(async () => {
		window.__app.activateRibbonTab('format')
		await new Promise((r) => setTimeout(r, 150))
		document.querySelector('[data-command="mermaid"]').click()
		await new Promise((r) => setTimeout(r, 900))
		const box = (sel) => {
			const el = document.querySelector(sel)
			if (!el) throw new Error('missing ' + sel)
			const r = el.getBoundingClientRect()
			return { L: r.left, R: r.right, T: r.top, B: r.bottom }
		}
		return {
			Fields: box('.diagram-fields'),
			Input: box('.diagram-fields input'),
			Preview: box('.diagram-preview'),
			SVG: box('.diagram-preview svg'),
		}
	})()`, &m)

	t.Logf("fields  %.0f..%.0f", m.Fields.L, m.Fields.R)
	t.Logf("input   %.0f..%.0f", m.Input.L, m.Input.R)
	t.Logf("preview %.0f..%.0f  x  %.0f..%.0f", m.Preview.L, m.Preview.R, m.Preview.T, m.Preview.B)
	t.Logf("svg     %.0f..%.0f  x  %.0f..%.0f", m.SVG.L, m.SVG.R, m.SVG.T, m.SVG.B)

	// A field must not reach past the column that holds it, and must never reach
	// the preview: that is the overlap the report describes.
	if m.Input.R > m.Fields.R+0.5 {
		t.Errorf("a field input ends at %.0f but its column ends at %.0f: it is %.0fpx wider "+
			"than the space it has", m.Input.R, m.Fields.R, m.Input.R-m.Fields.R)
	}
	if m.Input.R > m.Preview.L {
		t.Errorf("a field input ends at %.0f and the preview starts at %.0f: the input runs "+
			"underneath the preview", m.Input.R, m.Preview.L)
	}

	// The diagram must be inside the box that exists to contain it. The top edge
	// matters most: content above a scroll container's top cannot be reached.
	if m.SVG.T < m.Preview.T-0.5 {
		t.Errorf("the diagram starts %.0fpx above its preview box and cannot be scrolled to",
			m.Preview.T-m.SVG.T)
	}
	if m.SVG.B > m.Preview.B+0.5 {
		t.Errorf("the diagram ends %.0fpx below its preview box", m.SVG.B-m.Preview.B)
	}
}
