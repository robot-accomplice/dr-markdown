package e2e

import "testing"

// A ribbon command reads as a toolbar control, and is still 32px to hit.
//
// Reported from real use: the ribbon buttons read as generic web styling rather
// than a desktop application's controls. Every command was an outlined, rounded,
// filled rectangle, so a ribbon row was a line of discrete pills.
//
// What tells a user something is a command in a toolbar is its placement in a
// group, not a box drawn around it. The border returns on hover and while
// pressed, where it carries information instead of decoration.
//
// The risk in removing a border is shrinking what you can click. That is what
// this measures: the VISIBLE edge goes, the 32px target stays.
func TestARibbonCommandIsBorderlessAndStillFullSize(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	type control struct {
		Label   string  `json:"label"`
		Height  float64 `json:"height"`
		Border  string  `json:"border"`
		BgColor string  `json:"bg"`
	}
	var buttons []control
	var selects []control
	evalJS(t, ctx, `(async () => {
		window.__app.activateRibbonTab('format')
		await new Promise((r) => setTimeout(r, 300))
		const read = (el) => {
			const cs = getComputedStyle(el)
			return { label: (el.textContent || el.tagName).trim().slice(0, 18),
			         height: el.getBoundingClientRect().height,
			         border: cs.borderTopColor, bg: cs.backgroundColor }
		}
		return Array.from(document.querySelectorAll(
			'.ribbon-panel:not([hidden]) .ribbon-controls button')).slice(0, 10).map(read)
	})()`, &buttons)
	evalJS(t, ctx, `Array.from(document.querySelectorAll(
		'.ribbon-panel:not([hidden]) .ribbon-controls select')).slice(0, 4).map((el) => {
			const cs = getComputedStyle(el)
			return { label: 'select', height: el.getBoundingClientRect().height,
			         border: cs.borderTopColor, bg: cs.backgroundColor }
		})`, &selects)

	if len(buttons) == 0 {
		t.Fatal("no ribbon commands found to measure")
	}
	transparent := func(c string) bool {
		return c == "rgba(0, 0, 0, 0)" || c == "transparent"
	}
	for _, b := range buttons {
		t.Logf("  %-18s h=%.0f border=%s bg=%s", b.Label, b.Height, b.Border, b.BgColor)
		if b.Height < 32 {
			t.Errorf("%q is %.0fpx tall: removing the border must shrink the visible edge, "+
				"not the thing you have to hit", b.Label, b.Height)
		}
		if !transparent(b.Border) {
			t.Errorf("%q still draws a resting border (%s): a toolbar command is borderless "+
				"until you touch it", b.Label, b.Border)
		}
		if !transparent(b.BgColor) {
			t.Errorf("%q still fills at rest (%s), so the row remains a line of pills",
				b.Label, b.BgColor)
		}
	}
	// A select must keep an edge: nothing else about it says it opens.
	for _, s := range selects {
		t.Logf("  select             h=%.0f border=%s", s.Height, s.Border)
		if transparent(s.Border) {
			t.Errorf("a ribbon select lost its border: it has to read as something you can " +
				"open, and nothing else about it says so")
		}
	}
}
