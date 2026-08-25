package e2e

import (
	"strconv"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// The floating contextual bar is gone (#81). It was parked over the top of the
// document at a fixed offset, and for code and diagrams it appeared because the
// document CONTAINED one, with the caret nowhere near it — so it was never
// contextual, and its code control resolved the FIRST matching fence, editing
// the wrong block whenever a document had two.
//
// This gate holds the line that mattered in it: a per-block action must act on
// the block it was invoked from. The control now lives on the diagram itself.
func TestDiagramAssistantTargetsTheDiagramItWasOpenedFrom(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{
		"# Diagrams", "",
		"```mermaid", "graph TD", "  A[First] --> B[Keep]", "```", "",
		"```mermaid", "graph TD", "  C[Second] --> D[Replace]", "```", "",
	}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	if !waitForJS(t, ctx, `document.querySelectorAll('#wysiwyg .mermaid-render .diagram-edit').length === 2`) {
		t.Fatal("each rendered diagram should carry its own Edit control")
	}

	// Open the assistant from the SECOND diagram.
	evalJS(t, ctx, `document.querySelectorAll('#wysiwyg .mermaid-render .diagram-edit')[1].id = 'zz-second-edit'; 'ok'`, &res)
	if err := chromedp.Run(ctx, chromedp.Click("#zz-second-edit", chromedp.ByQuery)); err != nil {
		t.Fatalf("opening the assistant from the second diagram: %v", err)
	}

	var editIndex bool
	evalJS(t, ctx, `document.querySelector('[data-diagram-assistant][data-diagram-edit-index="1"]') !== null`, &editIndex)
	if !editIndex {
		var got string
		evalJS(t, ctx, `(() => { const a = document.querySelector('[data-diagram-assistant]');
			return a ? String(a.dataset.diagramEditIndex) : '(no assistant opened)' })()`, &got)
		t.Fatalf("the assistant should be editing fence index 1, the diagram it was opened from; got %s", got)
	}

	evalJS(t, ctx, `(() => {
		const input = document.querySelector('[data-diagram-field="yes"]')
		input.value = 'Updated'
		input.dispatchEvent(new Event('input', { bubbles: true }))
		document.querySelector('[data-diagram-action="insert"]').click()
		return 'ok'
	})()`, &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "A[First] --> B[Keep]") {
		t.Errorf("editing the second diagram altered the first:\n%s", md)
	}
	if !strings.Contains(md, "C[Updated]") {
		t.Errorf("the second diagram was not the one edited:\n%s", md)
	}
	if strings.Count(md, "```mermaid") != 2 {
		t.Errorf("the edit should replace, not append, a fence:\n%s", md)
	}
}

// The bar itself must stay gone: it is easy to reintroduce by restoring a call
// site, and nothing else would fail if it came back.
func TestNoFloatingContextualBarIsRendered(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{
		"# Doc", "",
		"| Name | Value |", "| --- | --- |", "| Alpha | 1 |", "",
		"```python", "x = 1", "```", "",
		"```mermaid", "graph TD", "  A --> B", "```", "",
	}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block') !== null`) {
		t.Fatal("the document never rendered")
	}

	var present bool
	evalJS(t, ctx, `document.querySelector('[data-contextual-controls]') !== null ||
		document.getElementById('contextual-controls-root') !== null ||
		document.querySelector('[data-context-group]') !== null`, &present)
	if present {
		t.Error("a floating contextual controls bar is being rendered again: per-block " +
			"actions belong on their block, not in a bar parked over the document (#81)")
	}
}
