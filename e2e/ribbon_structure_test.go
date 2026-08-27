package e2e

import (
	"strings"
	"testing"
)

// No command appears in more than one ribbon tab.
//
// The ribbon had a Home tab that was a SUPERSET of two others: bold, italic,
// strike and quote were in Home and Format; image, table, code-block, math and
// mermaid were in Home and Insert. Format and Insert were strict subsets of
// Home, so two of the five tabs taught the user nothing new, and a change to a
// control had two places to be made.
//
// Home also carried an empty Share group, and the file operations had no ribbon
// home at all — Save and Save As existed as HIDDEN buttons outside it, present
// only to be a click target for the menu bar.
//
// This gate holds the property that made the restructure worth doing, rather
// than the specific arrangement: any tab may gain or lose a control, so long as
// no control is in two places.
func TestNoRibbonCommandAppearsInTwoTabs(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var duplicates string
	evalJS(t, ctx, `(() => {
		const seen = new Map()
		const dupes = []
		for (const panel of document.querySelectorAll('[data-ribbon-panel]')) {
			const tab = panel.dataset.ribbonPanel
			for (const b of panel.querySelectorAll('[data-command], [data-file-action], [data-insert-command]')) {
				const key = b.dataset.command || b.dataset.fileAction || b.dataset.insertCommand
				if (seen.has(key) && seen.get(key) !== tab) {
					dupes.push(key + ' in both ' + seen.get(key) + ' and ' + tab)
				} else {
					seen.set(key, tab)
				}
			}
		}
		return JSON.stringify(dupes)
	})()`, &duplicates)

	if duplicates != "[]" {
		t.Errorf("commands appear in more than one ribbon tab, so a change to one has two "+
			"places to be made:\n  %s", strings.ReplaceAll(duplicates, "\",\"", "\n  "))
	}
}

// Every ribbon group must contain something. Home shipped an empty "Share"
// group heading with no controls under it.
func TestNoRibbonGroupIsEmpty(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var empty string
	evalJS(t, ctx, `(() => {
		const bad = []
		for (const panel of document.querySelectorAll('[data-ribbon-panel]')) {
			for (const g of panel.querySelectorAll('.ribbon-group')) {
				if (g.querySelectorAll('button, select').length === 0) {
					const h = g.querySelector('h2')
					bad.push(panel.dataset.ribbonPanel + ' / ' + (h ? h.textContent : '(unnamed)'))
				}
			}
		}
		return JSON.stringify(bad)
	})()`, &empty)

	if empty != "[]" {
		t.Errorf("ribbon groups with a heading and no controls: %s", empty)
	}
}

// The file operations are reachable from the ribbon, and still work from the
// places they already lived.
func TestFileOperationsAreReachableFromTheRibbon(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var out struct {
		HasFileTab bool     `json:"hasFileTab"`
		HasHomeTab bool     `json:"hasHomeTab"`
		Actions    []string `json:"actions"`
		Unwired    []string `json:"unwired"`
	}
	evalJS(t, ctx, `(async () => {
		// File is a MENU now, not a tab: every other tab changes what the ribbon
		// offers for the document you are editing, and File acts on the document
		// as a whole. Its commands moved rather than disappeared, so this still
		// checks that every one of them is present and wired.
		document.getElementById('btn-file-menu').click()
		await new Promise((r) => setTimeout(r, 250))
		const menu = document.querySelector('[data-file-menu]')
		const actions = menu ? Array.from(menu.querySelectorAll('[data-file-menu-action]'))
			.map((b) => b.dataset.fileMenuAction) : []
		// Every action in the document must be one the app knows how to run; a
		// button wired to nothing is the defect this app has a rule against.
		const known = ['new', 'open', 'save', 'save-as', 'print', 'pdf']
		const unwired = Array.from(document.querySelectorAll('[data-file-menu-action], [data-file-action]'))
			.map((b) => b.dataset.fileMenuAction || b.dataset.fileAction)
			.filter((a) => !known.includes(a))
		return {
			hasFileTab: document.querySelector('[data-ribbon-panel="file"]') !== null,
			hasHomeTab: document.querySelector('[data-ribbon-panel="home"]') !== null,
			actions,
			unwired: [...new Set(unwired)],
		}
	})()`, &out)

	if out.HasFileTab {
		t.Error("File is still a ribbon tab: it acts on the document as a whole, not on " +
			"what you are editing, so it belongs in a menu")
	}
	if out.HasHomeTab {
		t.Error("the Home tab should be gone: it duplicated Format and Insert entirely")
	}
	for _, want := range []string{"new", "open", "save", "save-as", "print", "pdf"} {
		found := false
		for _, got := range out.Actions {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the File menu should offer %q; it has %v", want, out.Actions)
		}
	}
	if len(out.Unwired) > 0 {
		t.Errorf("file actions the app cannot run: %v", out.Unwired)
	}
}

// The tab marked active at boot must have a visible panel.
//
// activateRibbonTab is only called on click — nothing runs it at startup — so
// the default panel's visibility comes from the markup alone. Adding the File
// panel with `hidden` set, while its tab button carried `class="active"`,
// rendered an EMPTY ribbon on load: the whole control surface gone, and every
// command still present in the DOM so a presence check would not notice.
func TestTheActiveRibbonTabHasAVisiblePanelAtBoot(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var out struct {
		ActiveTab string `json:"activeTab"`
		Visible   int    `json:"visible"`
		Controls  int    `json:"controls"`
	}
	evalJS(t, ctx, `(() => {
		const active = document.querySelector('[data-ribbon-tab].active')
		const shown = Array.from(document.querySelectorAll('[data-ribbon-panel]')).filter((p) => !p.hidden)
		return {
			activeTab: active ? active.dataset.ribbonTab : '(none)',
			visible: shown.length,
			controls: shown.reduce((n, p) => n + p.querySelectorAll('button, select').length, 0),
		}
	})()`, &out)

	if out.Visible != 1 {
		t.Errorf("exactly one ribbon panel should be visible at boot, %d are", out.Visible)
	}
	if out.Controls == 0 {
		t.Errorf("the ribbon is empty at boot: tab %q is marked active but its panel is hidden, "+
			"and nothing unhides it until a tab is clicked", out.ActiveTab)
	}
}
