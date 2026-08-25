package e2e

import (
	"strconv"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// The same code, viewed three ways, must look the same.
//
// Reported from real use: font, colours, syntax highlighting and tab behaviour
// all differed between Formatted, Raw and Split, and it was most obvious when
// entering code. Measured before the fix, for one fenced Go block:
//
//	surface     font              size     tab-size   keyword
//	formatted   monospace         15.5px   4          rgb(198,120,221)
//	split       JetBrains Mono    12.5px   8          oklch(0.52 0.09 196)
//	raw         JetBrains Mono    12.5px   2          oklch(0.52 0.09 196)
//
// Three tab widths, two font families, and a keyword colour from CodeMirror's
// One Dark — a DARK theme's foreground on a white block, because app.css had
// suppressed that theme's background and left its colours alone.
//
// This gate compares the surfaces against each other rather than against fixed
// values, because the defect is disagreement: any of them may change, so long
// as they change together.
func TestCodeLooksTheSameInEverySurface(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{"# Doc", "", "```go", `func main() { s := "hi" }`, "```", ""}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .cm-content') !== null`) {
		t.Fatal("the formatted code surface never mounted")
	}

	read := `(sel) => {
		const e = document.querySelector(sel)
		if (!e) return null
		const cs = getComputedStyle(e)
		return { font: cs.fontFamily.split(',')[0].replace(/"/g, ''), size: cs.fontSize, tab: cs.tabSize }
	}`

	var formatted, split map[string]string
	evalJS(t, ctx, `(`+read+`)('#wysiwyg .cm-content')`, &formatted)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)
	evalJS(t, ctx, `(`+read+`)('#split-preview .code-block-shell code')`, &split)
	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)
	var raw map[string]string
	evalJS(t, ctx, `(`+read+`)('#raw textarea')`, &raw)

	for _, key := range []string{"font", "size", "tab"} {
		if formatted[key] != split[key] || split[key] != raw[key] {
			t.Errorf("%s differs between surfaces: formatted=%q split=%q raw=%q",
				key, formatted[key], split[key], raw[key])
		}
	}
}

// The formatted editor's tokens must come from the app's palette, not from the
// vendored One Dark theme it ships with.
func TestFormattedCodeUsesTheAppSyntaxPalette(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{"```go", `func main() { s := "hi" }`, "```", ""}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .cm-content span[class]') !== null`) {
		t.Fatal("the formatted code surface never highlighted anything")
	}

	var offenders string
	evalJS(t, ctx, `(() => {
		// One Dark's signature colours, as rgb() — what a browser reports for
		// the hex literals the vendored theme used to carry.
		const oneDark = {
			'rgb(198, 120, 221)': 'keyword purple',
			'rgb(97, 175, 239)':  'function blue',
			'rgb(171, 178, 191)': 'foreground grey',
			'rgb(152, 195, 121)': 'string green',
			'rgb(224, 108, 117)': 'variable red',
			'rgb(86, 182, 194)':  'operator cyan',
		}
		const bad = []
		for (const s of document.querySelectorAll('#wysiwyg .cm-content span[class], #wysiwyg .cm-content, #wysiwyg .cm-line')) {
			const c = getComputedStyle(s).color
			if (oneDark[c]) bad.push(JSON.stringify(s.textContent.slice(0, 12)) + ' is ' + oneDark[c] + ' ' + c)
		}
		return JSON.stringify([...new Set(bad)])
	})()`, &offenders)

	if offenders != "[]" {
		t.Errorf("the formatted editor is drawing CodeMirror's One Dark palette on a light "+
			"surface instead of the app's syntax tokens:\n  %s", offenders)
	}
}

// Tab indents in the raw editor instead of leaving it.
func TestTabIndentsInTheRawEditor(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote("alpha\n")+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#raw textarea') !== null`) {
		t.Fatal("the raw editor never mounted")
	}

	// Put the caret on the "alpha" line explicitly. Clicking lands it at the end
	// of the document, which for "alpha\n" is the empty SECOND line — Tab
	// indents that correctly and the assertion below would be measuring the
	// wrong line.
	clickWhenVisible(t, ctx, "#raw textarea")
	var placed bool
	evalJS(t, ctx, `(() => {
		const a = document.querySelector('#raw textarea')
		a.focus()
		a.setSelectionRange(0, 0)
		return a.selectionStart === 0
	})()`, &placed)
	if !placed {
		t.Fatal("could not place the caret at the start of the first line")
	}
	if err := chromedp.Run(ctx, chromedp.KeyEvent(kb.Tab)); err != nil {
		t.Fatalf("pressing Tab: %v", err)
	}

	var out struct {
		Value   string `json:"value"`
		Focused bool   `json:"focused"`
	}
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 300))
		const a = document.querySelector('#raw textarea')
		return { value: a.value, focused: document.activeElement === a }
	})()`, &out)

	if !out.Focused {
		t.Error("Tab moved focus out of the raw editor: a source editor that cannot be " +
			"indented is the defect, and the formatted view indents on the same key")
	}
	if !strings.HasPrefix(out.Value, "    alpha") {
		t.Errorf("Tab did not indent the line the caret was on, value is %q", out.Value)
	}
}
