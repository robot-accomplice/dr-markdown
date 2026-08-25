package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// Crepe's block-edit affordances — the hover handle and the slash menu — take
// their highlight colours from `theme/common/block-edit.css`, which knows
// nothing about this app's accent (#76). Every colour in that file comes from a
// `--crepe-color-*` variable, so rebinding the palette at its source answers
// this too, rather than needing a rule per control.
//
// This gate asserts on the HANDLE ELEMENT, not on `.milkdown`. The remap is
// scoped to `#wysiwyg .milkdown` and reaches these controls only because they
// are rendered inside it — measured, because a menu portalled to document.body
// would inherit nothing and the app would be back to vendored brown with every
// other palette test still green.
func TestBlockEditMenuTakesItsHighlightFromTheAppAccent(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote("# Title\n\nA paragraph to hover.\n")+").then(() => 'ok')", &res)

	var out struct {
		HandleFound  bool   `json:"handleFound"`
		HandleInside bool   `json:"handleInside"`
		MenuInside   bool   `json:"menuInside"`
		Hover        string `json:"hover"`
		Selected     string `json:"selected"`
		AccentWash   string `json:"accentWash"`
		AccentBorder string `json:"accentBorder"`
	}
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 500))
		const p = document.querySelector('#wysiwyg .ProseMirror p')
		if (p) {
			const r = p.getBoundingClientRect()
			for (const type of ['pointerover', 'pointerenter', 'mouseover', 'mousemove']) {
				p.dispatchEvent(new MouseEvent(type, { bubbles: true, clientX: r.left + 5, clientY: r.top + 5 }))
			}
		}
		await new Promise((r) => setTimeout(r, 500))
		const handle = document.querySelector('.milkdown-block-handle')
		const menu = document.querySelector('.milkdown-slash-menu')
		const root = getComputedStyle(document.documentElement)
		const hs = handle ? getComputedStyle(handle) : null
		return {
			handleFound: !!handle,
			handleInside: !!(handle && handle.closest('#wysiwyg .milkdown')),
			menuInside: !!(menu && menu.closest('#wysiwyg .milkdown')),
			hover: hs ? hs.getPropertyValue('--crepe-color-hover').trim() : '',
			selected: hs ? hs.getPropertyValue('--crepe-color-selected').trim() : '',
			accentWash: root.getPropertyValue('--accent-wash').trim(),
			accentBorder: root.getPropertyValue('--accent-wash-border').trim(),
		}
	})()`, &out)

	if !out.HandleFound {
		t.Fatal("no block handle appeared on hover, so this gate measured nothing")
	}
	if !out.HandleInside {
		t.Error("the block handle is not inside #wysiwyg .milkdown, so it inherits none of the " +
			"app's palette rebinding: it will draw in the vendored warm colours")
	}
	if !out.MenuInside {
		t.Error("the slash menu is not inside #wysiwyg .milkdown, so it inherits none of the " +
			"app's palette rebinding")
	}
	if out.Hover != out.AccentWash {
		t.Errorf("block-edit hover resolves to %q, want the app's --accent-wash %q (#76)",
			out.Hover, out.AccentWash)
	}
	if out.Selected != out.AccentBorder {
		t.Errorf("block-edit selected resolves to %q, want the app's --accent-wash-border %q (#76)",
			out.Selected, out.AccentBorder)
	}
	if strings.Contains(out.Hover, "#") || strings.Contains(out.Selected, "#") {
		t.Errorf("block-edit highlight still carries a literal vendored hex (hover=%q selected=%q)",
			out.Hover, out.Selected)
	}
}
