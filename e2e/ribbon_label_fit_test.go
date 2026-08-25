package e2e

import (
	"strings"
	"testing"
)

// No ribbon button may render its label wider than the button itself.
//
// The labelled ribbon controls were laid out with a FIXED width per button —
// 78px by default — plus a hand-computed exception for every label that did not
// fit: table, code-block, math, mermaid, hr, quote, strike, focus, and two more
// by id, repeated again at a second breakpoint. Twelve rules, each added when
// someone noticed a specific label clipping.
//
// The Help panel's two buttons were never given one, so "Markdown help" and
// "Keyboard shortcuts" overflowed their 78px boxes and drew ON TOP of each
// other. That is what a fixed width with a growing exception list produces: the
// next label anyone adds is broken until it is noticed by eye.
//
// So this gate measures every button in every ribbon tab, rather than the ones
// someone remembered. It compares scrollWidth against clientWidth, which is the
// only check that fails for a label that does not fit — a presence check cannot,
// and neither can a screenshot nobody looks at.
func TestNoRibbonButtonLabelOverflowsItsButton(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Doc\\n').then(() => 'ok')", &res)

	var overflowing string
	evalJS(t, ctx, `(async () => {
		const bad = []
		const tabs = Array.from(document.querySelectorAll('[data-ribbon-tab]')).map((t) => t.dataset.ribbonTab)
		for (const tab of tabs) {
			window.__app.activateRibbonTab(tab)
			await new Promise((r) => setTimeout(r, 60))
			const panel = document.querySelector('[data-ribbon-panel="' + tab + '"]')
			if (!panel || panel.hidden) continue
			for (const b of panel.querySelectorAll('button, select')) {
				const r = b.getBoundingClientRect()
				if (r.width === 0) continue
				// A hair of tolerance: sub-pixel text metrics round up by design.
				if (b.scrollWidth > b.clientWidth + 1) {
					bad.push(tab + ' / "' + b.textContent.trim() + '" content ' + b.scrollWidth +
						'px in a ' + b.clientWidth + 'px box')
				}
			}
		}
		return JSON.stringify(bad)
	})()`, &overflowing)

	if overflowing != "[]" {
		t.Errorf("ribbon labels do not fit their buttons, so they clip or overlap:\n  %s",
			strings.ReplaceAll(strings.Trim(overflowing, "[]"), "\",\"", "\n  "))
	}
}
