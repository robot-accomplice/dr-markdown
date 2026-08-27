package e2e

import (
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// headerPadding mirrors `.code-block-header { padding: 0 12px }` in app.css.
// The gate compares against the design value rather than against whatever the
// browser happens to compute, so changing the CSS deliberately means changing
// this line deliberately.
const headerPadding = 12

// The app's code-block shell is a PREVIEW and PRINT construct now. It used to
// be injected into the editor as well, which is where this test used to point.
//
// Why it pointed there, and what that caught: an existing test asserted the
// language label and Copy button EXIST and carry the right text, and it passed
// for the whole time the chrome was visibly wrong — the label sat against the
// block's left border with no padding, and Copy sat inline beside it instead of
// at the right edge. The cause was a specificity tie. `.milkdown *` in the
// vendored Crepe reset sets `margin: 0; padding: 0`, and `.milkdown *` and
// `.code-block-header` are both (0,1,0); index.html loads app.css in <head>
// while loadTheme() appends the vendored sheets at runtime, so the vendored
// rule is later and later wins a tie.
//
// That particular collision can no longer happen, because the app no longer
// injects anything into the editor: the editor's own node view draws code
// blocks, and drawing over it is what left them uneditable (#77). The lesson it
// records is still live for anything else placed inside `.milkdown`, and the
// geometry check is still the only thing that can fail for a layout fault
// rather than a missing element — so it keeps doing that job on the surface
// where the shell still renders.
func TestCodeBlockChromeIsLaidOutAsDesigned(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "```go\nfunc main() {}\n```\n"
	// Split shows the real editor now, so the app-injected shell survives only
	// on the print surface.
	renderToPrintRoot(t, ctx, fixture)

	if !waitForJS(t, ctx, `document.querySelector('#print-root .code-block-shell .code-block-header') !== null`) {
		t.Fatal("no code block chrome rendered on the print surface")
	}

	// The shell is laid out only under print media: #print-root is display:none
	// on screen, and a display:none element measures 0 for every rect — which is
	// how this check silently reported "jammed against the border" while the CSS
	// was untouched. Computed style still resolves on a hidden element, which is
	// why the padding assertion below kept passing and the geometry did not.
	if err := chromedp.Run(ctx, emulation.SetEmulatedMedia().WithMedia("print")); err != nil {
		t.Fatalf("emulating print media: %v", err)
	}
	defer chromedp.Run(ctx, emulation.SetEmulatedMedia().WithMedia("screen"))

	var m struct {
		LabelInset int `json:"labelInset"`
		CopyInset  int `json:"copyInset"`
		HeaderPadL int `json:"headerPadL"`
	}
	evalJS(t, ctx, `(() => {
		const shell = document.querySelector('#print-root .code-block-shell')
		const label = shell.querySelector('.code-block-language')
		const copy = shell.querySelector('.code-block-copy')
		const s = shell.getBoundingClientRect()
		return {
			labelInset: Math.round(label.getBoundingClientRect().left - s.left),
			copyInset: Math.round(s.right - copy.getBoundingClientRect().right),
			headerPadL: parseInt(getComputedStyle(shell.querySelector('.code-block-header')).paddingLeft, 10),
		}
	})()`, &m)

	if m.HeaderPadL != headerPadding {
		t.Errorf("header padding-left computed %dpx, designed %dpx: a rule is winning the cascade",
			m.HeaderPadL, headerPadding)
	}
	// One border pixel sits between the shell edge and the header's padding box.
	const tolerance = 2
	if abs(m.LabelInset-headerPadding) > tolerance {
		t.Errorf("language label sits %dpx from the block's left edge, designed %dpx: "+
			"it is jammed against the border", m.LabelInset, headerPadding)
	}
	if abs(m.CopyInset-headerPadding) > tolerance {
		t.Errorf("Copy sits %dpx from the block's right edge, designed %dpx: "+
			"`margin-right: auto` on the label is not pushing it right, so the chrome reads "+
			"as two labels crowded on the left", m.CopyInset, headerPadding)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
