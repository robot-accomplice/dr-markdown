package e2e

import (
	"strconv"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// The editor's slash menu must actually insert what it offers, and what it
// inserts must be editable (#75, #77).
//
// Two instrument traps are baked into how this is written, because both cost
// real time and both produce a CONVINCING false failure:
//
//  1. Synthetic clicks do not drive this menu. Dispatching pointerdown /
//     mousedown / mouseup / click at the item's own centre leaves the item's
//     onRun uninvoked and the document unchanged — measured, with a counter
//     inside the vendored handler. A real browser click on the same element
//     invokes it and inserts the block. #75 was reported with evidence of
//     exactly the synthetic shape: 18 items found, Code found, document
//     unchanged, no errors. So this gate uses chromedp.Click, never dispatch.
//
//  2. The document must not be empty. On an empty document the app shows its
//     empty state instead of the editor, so `#wysiwyg .ProseMirror` exists but
//     is not visible, and chromedp.Click waits on it until the test times out.
//
// Crepe opens the menu only for "/" in an EMPTY block, hence the Enter first.
func TestSlashMenuInsertsAnEditableCodeBlock(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote("para\n")+").then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .ProseMirror p') !== null`) {
		t.Fatal("the formatted surface never mounted")
	}

	clickWhenVisible(t, ctx, "#wysiwyg .ProseMirror p")
	if err := chromedp.Run(ctx,
		chromedp.KeyEvent(kb.End),
		chromedp.KeyEvent(kb.Enter),
		chromedp.KeyEvent("/"),
	); err != nil {
		t.Fatalf("opening the slash menu: %v", err)
	}

	var found bool
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 800))
		const menu = document.querySelector('#wysiwyg .milkdown-slash-menu')
		if (!menu || menu.getAttribute('data-show') === 'false') return false
		const item = Array.from(menu.querySelectorAll('.milkdown-menu-item, li, button'))
			.filter((i) => i.getBoundingClientRect().height > 0)
			.find((i) => i.textContent.trim().toLowerCase().startsWith('code'))
		if (!item) return false
		item.id = 'slash-code-item'
		window.__beforeSlash = window.__app.getMarkdown()
		return true
	})()`, &found)
	if !found {
		t.Fatal("typing / in an empty block should open the slash menu with a Code item")
	}

	clickWhenVisible(t, ctx, "#slash-code-item")

	var after struct {
		Before   string `json:"before"`
		After    string `json:"after"`
		Editable bool   `json:"editable"`
	}
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 1000))
		return {
			before: window.__beforeSlash,
			after: window.__app.getMarkdown(),
			editable: document.querySelector('#wysiwyg .cm-content[contenteditable="true"]') !== null,
		}
	})()`, &after)

	if after.Before == after.After {
		t.Errorf("choosing Code from the slash menu left the document unchanged (#75):\n%s", after.After)
	}
	if !strings.Contains(after.After, "```") {
		t.Errorf("choosing Code did not insert a fenced block:\n%s", after.After)
	}
	// An inserted block that cannot be typed into is the #77 defect, and it is
	// also what made #75 look like "nothing happened" from the user's seat.
	if !after.Editable {
		t.Error("the inserted code block has no editable surface: an insert that " +
			"produces a box you cannot type into reads as a broken insert")
	}
}
