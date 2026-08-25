package e2e

import (
	"strconv"
	"testing"
)

// The document's first line sits in the same place whatever it is formatted as.
//
// Reported from real use: the top line moves down or up depending on whether
// heading format is applied. Measured — for a one-block document, the first
// block's top edge landed at:
//
//	paragraph  199    (margin-top 0px)
//	heading 1  231    (margin-top 32px)
//	heading 2  227    (margin-top 28px)
//
// The heading's own top margin collapsed OUT through `.ProseMirror` and
// `.milkdown`, which both have margin, padding and border of zero and so cannot
// contain it. `#wysiwyg`'s 52px padding stopped it going further, which is why
// it surfaced as the whole editor sitting lower rather than as space inside it.
//
// Applying a heading to the first line should change the line, not move it.
func TestFirstLineDoesNotMoveWhenItBecomesAHeading(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	top := func(md string) int {
		var res string
		evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(md)+").then(() => 'ok')", &res)
		var t0 int
		evalJS(t, ctx, `(async () => {
			await new Promise((r) => setTimeout(r, 400))
			const pm = document.querySelector('#wysiwyg .ProseMirror')
			const block = Array.from(pm.children).find((c) => !c.classList.contains('ProseMirror-widget'))
			const host = document.querySelector('#wysiwyg')
			return Math.round(block.getBoundingClientRect().top - host.getBoundingClientRect().top)
		})()`, &t0)
		return t0
	}

	paragraph := top("plain paragraph\n")
	h1 := top("# heading one\n")
	h2 := top("## heading two\n")
	h3 := top("### heading three\n")

	t.Logf("first block top, relative to the editor pane: paragraph=%d h1=%d h2=%d h3=%d",
		paragraph, h1, h2, h3)

	for _, c := range []struct {
		name string
		got  int
	}{{"heading 1", h1}, {"heading 2", h2}, {"heading 3", h3}} {
		if c.got != paragraph {
			t.Errorf("%s starts %dpx from the top of the pane but a paragraph starts %dpx: "+
				"applying a heading to the first line moves it by %dpx",
				c.name, c.got, paragraph, c.got-paragraph)
		}
	}
}
