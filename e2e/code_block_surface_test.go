package e2e

import (
	"strconv"
	"testing"
)

// The design values a formatted code block's surface must carry, mirroring
// `#wysiwyg .milkdown-code-block` in app.css. Compared against the design
// rather than against whatever the browser computes, so changing the CSS means
// changing this deliberately.
const (
	blockCornerRadius = 9
	blockBorderWidth  = 1
)

// The editor's own code block IS the box now that the app no longer draws a
// shell over it, so it has to carry the box's design itself.
//
// This is a layout fault a presence check cannot see, and one that actually
// happened: cancelling the vendored theme's warm tint without replacing the
// surface left the block with no border, no radius, a transparent background
// and a peach CodeMirror inside it — while every test asserting that code
// blocks exist, highlight and accept typing stayed green.
func TestFormattedCodeBlockCarriesTheBlockSurface(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "Intro\n\n```go\nfunc main() {}\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .cm-content') !== null`) {
		t.Fatal("no code block mounted in the formatted surface")
	}

	var m struct {
		Radius       int    `json:"radius"`
		BorderWidth  int    `json:"borderWidth"`
		Background   string `json:"background"`
		CMBackground string `json:"cmBackground"`
	}
	evalJS(t, ctx, `(() => {
		const block = document.querySelector('#wysiwyg .milkdown-code-block')
		const cs = getComputedStyle(block)
		return {
			radius: parseInt(cs.borderTopLeftRadius, 10),
			borderWidth: parseInt(cs.borderTopWidth, 10),
			background: cs.backgroundColor,
			cmBackground: getComputedStyle(block.querySelector('.cm-editor')).backgroundColor,
		}
	})()`, &m)

	if m.Radius != blockCornerRadius {
		t.Errorf("code block corner radius computed %dpx, designed %dpx", m.Radius, blockCornerRadius)
	}
	if m.BorderWidth != blockBorderWidth {
		t.Errorf("code block border computed %dpx, designed %dpx: the block has no edge, "+
			"so it reads as loose text rather than a card", m.BorderWidth, blockBorderWidth)
	}
	// A transparent block means the surface was cancelled and never replaced.
	if m.Background == "rgba(0, 0, 0, 0)" || m.Background == "transparent" {
		t.Errorf("code block background is %s: the block supplies no surface of its own", m.Background)
	}
	// The vendored theme tints CodeMirror warm; the block's own surface must win.
	if m.CMBackground != "rgba(0, 0, 0, 0)" && m.CMBackground != m.Background {
		t.Errorf("CodeMirror inside the block is %s while the block is %s: the vendored warm "+
			"tint is showing through, which is what made the card read peach", m.CMBackground, m.Background)
	}
}
