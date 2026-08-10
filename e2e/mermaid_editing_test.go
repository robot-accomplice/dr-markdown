package e2e

import (
	"strconv"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// Mermaid fences render as diagrams in Formatted mode. Before the code-mirror
// repair (#77) the app produced that diagram by replacing the editor's own
// <pre>, which meant the diagram was inert: there was no way to edit the
// source without leaving Formatted mode. That is the same rule violation as
// #77 in a different construct — WYSIWYG means everything renders AND edits in
// place (docs/architext/data/rules.json, `wysiwyg-is-the-purpose`).
//
// Crepe's code-mirror node view supports exactly this shape: a rendered
// preview with a toggle back to the editable source. This gate requires both
// halves, so neither can be lost without failing.
func TestMermaidRendersAndStaysEditableInFormattedMode(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "```mermaid\ngraph TD\n  A[Start] --> B[Finish]\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	// Half one: the diagram renders.
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .mermaid-render svg') !== null`) {
		t.Fatal("formatted mode should render mermaid fences as diagrams")
	}

	// Half two: the source behind the diagram is reachable and editable.
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .preview-toggle-button') !== null`) {
		t.Fatal("a rendered mermaid diagram should offer a way back to its source: " +
			"a diagram that cannot be edited in place is the #77 defect in another construct")
	}
	if err := chromedp.Run(ctx,
		chromedp.Click("#wysiwyg .milkdown-code-block .preview-toggle-button", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("toggling a mermaid diagram back to source: %v", err)
	}
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .cm-content[contenteditable="true"]') !== null`) {
		t.Fatal("toggling a mermaid diagram should expose an editable source surface")
	}

	const typed = "  B --> C[Done]"
	if err := chromedp.Run(ctx,
		chromedp.Click("#wysiwyg .cm-content", chromedp.ByQuery),
		chromedp.SendKeys("#wysiwyg .cm-content", "\n"+typed, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("typing into a mermaid source: %v", err)
	}

	var got string
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 500))
		return window.__app.getMarkdown()
	})()`, &got)

	if !strings.Contains(got, "C[Done]") {
		t.Errorf("edits to a mermaid diagram's source never reached the document:\n%s", got)
	}
	if !strings.Contains(got, "```mermaid") {
		t.Errorf("the mermaid fence did not survive editing:\n%s", got)
	}
}
