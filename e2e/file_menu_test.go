package e2e

import "testing"

// File is a menu, and opening it costs you nothing.
//
// Reported from real use: File should not be a ribbon tab. Every other tab
// changes what the ribbon offers for the document you are editing; File acts on
// the document as a whole and none of its commands is something you reach for
// while writing. The consequence was that the application opened showing
// commands you use once a session, with the formatting controls you use
// constantly behind another tab.
func TestFileIsAMenuAndDoesNotDisturbTheRibbon(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var boot struct {
		Tabs       []string `json:"tabs"`
		ActiveTab  string   `json:"activeTab"`
		HasFileTab bool     `json:"hasFileTab"`
	}
	evalJS(t, ctx, `(() => {
		const tabs = Array.from(document.querySelectorAll('[data-ribbon-tab]'))
		const active = tabs.find((t) => t.classList.contains('active'))
		return { tabs: tabs.map((t) => t.dataset.ribbonTab),
		         activeTab: active ? active.dataset.ribbonTab : '(none)',
		         hasFileTab: tabs.some((t) => t.dataset.ribbonTab === 'file') }
	})()`, &boot)
	t.Logf("tabs=%v active=%s", boot.Tabs, boot.ActiveTab)

	if boot.HasFileTab {
		t.Error("File is still a ribbon tab")
	}
	if boot.ActiveTab != "format" {
		t.Errorf("the ribbon opens on %q; it should open on the controls you use while "+
			"writing, not one click away from them", boot.ActiveTab)
	}

	// Opening File must leave the ribbon exactly where it was.
	var after struct {
		Open      bool     `json:"open"`
		Items     []string `json:"items"`
		ActiveTab string   `json:"activeTab"`
		Expanded  string   `json:"expanded"`
	}
	evalJS(t, ctx, `(async () => {
		document.getElementById('btn-file-menu').click()
		await new Promise((r) => setTimeout(r, 250))
		const menu = document.querySelector('[data-file-menu]')
		const active = Array.from(document.querySelectorAll('[data-ribbon-tab]'))
			.find((t) => t.classList.contains('active'))
		return {
			open: !!menu,
			items: menu ? Array.from(menu.querySelectorAll('[data-file-menu-action]'))
				.map((b) => b.dataset.fileMenuAction) : [],
			activeTab: active ? active.dataset.ribbonTab : '(none)',
			expanded: document.getElementById('btn-file-menu').getAttribute('aria-expanded'),
		}
	})()`, &after)
	t.Logf("menu open=%v items=%v ribbon still on %q", after.Open, after.Items, after.ActiveTab)

	if !after.Open {
		t.Fatal("clicking File opened no menu")
	}
	if after.ActiveTab != boot.ActiveTab {
		t.Errorf("opening File moved the ribbon from %q to %q: reaching File must not cost "+
			"you your place", boot.ActiveTab, after.ActiveTab)
	}
	if after.Expanded != "true" {
		t.Errorf("aria-expanded is %q while the menu is open", after.Expanded)
	}
	for _, want := range []string{"new", "open", "save", "save-as", "print", "pdf"} {
		found := false
		for _, got := range after.Items {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the File menu is missing %q; it carried that command as a tab", want)
		}
	}

	// And it closes on an outside click, because that is what a menu does.
	var closed bool
	evalJS(t, ctx, `(async () => {
		document.body.click()
		await new Promise((r) => setTimeout(r, 250))
		return !document.querySelector('[data-file-menu]')
	})()`, &closed)
	if !closed {
		t.Error("the File menu stayed open after a click elsewhere")
	}
}
