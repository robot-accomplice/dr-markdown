// Package e2e drives the vendored frontend in headless Chrome via chromedp.
// It serves frontend/dist over httptest (matching the app's asset-scheme
// environment) and talks to the page through the window.__app hooks.
//
// Requires a Chrome/Chromium binary on the machine. Tests FAIL rather than
// skip when none is found, because this package is the only coverage of the
// frontend and a silent skip once let the whole suite vanish while `go test`
// still printed ok. Set DRMD_SKIP_E2E to opt out deliberately. Pure Go — no
// Node involved.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func chromeAvailable() bool {
	if _, err := os.Stat("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"); err == nil {
		return true // macOS default install
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		// The `.exe` names matter now that a missing browser fails instead of
		// skipping: this is a three-platform product, and a Windows developer
		// with Chrome installed would otherwise get a hard failure telling them
		// to install what they already have.
		for _, name := range []string{
			"google-chrome", "chrome", "chromium", "chromium-browser", "chrome-headless-shell",
			"chrome.exe", "chromium.exe", "chrome-headless-shell.exe",
		} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return true
			}
		}
	}
	return false
}

// skipE2EEnv is the explicit, deliberate opt-out from browser coverage. It
// exists so that skipping is a decision someone recorded, never a default.
const skipE2EEnv = "DRMD_SKIP_E2E"

func newTestBrowser(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	// Fail, do not skip. Every test of this application's frontend logic lives
	// in this package and runs through Chrome, so a missing binary silently
	// removed the entire behavioural suite while `go test ./...` still printed
	// `ok` — a green board that had verified nothing. An environment that
	// genuinely cannot run a browser has to say so out loud.
	if !chromeAvailable() {
		if os.Getenv(skipE2EEnv) != "" {
			t.Skipf("no Chrome/Chromium binary found and %s is set", skipE2EEnv)
		}
		t.Fatalf("no Chrome/Chromium binary found: the e2e suite is the only "+
			"coverage of the frontend, so it must not pass by being absent. "+
			"Install Chrome, or set %s=1 to accept an unverified frontend.", skipE2EEnv)
	}
	// Chrome's setuid sandbox cannot initialize on a CI runner without extra
	// privileges — it aborts in ZygoteHostImpl::Init before any test runs. The
	// browser here only ever loads frontend/dist from a local httptest server,
	// so disabling the sandbox costs nothing this harness relies on.
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.NoSandbox)
	// Give Chrome longer to report its debugging websocket than chromedp's
	// 20s default, which is tuned for a developer machine launching one
	// browser.
	//
	// This suite launches one per test — 91 of them per run — and on a
	// two-core CI runner a launch under contention exceeded 20s twice, failing
	// with "websocket url timeout reached" before any page work. Both failures
	// were at connect, on commits that passed on re-run, and both followed this
	// session adding 17 launches.
	//
	// The real reduction is fewer browsers: the fidelity and markdown unit
	// tests boot a whole browser only to import() a pure module, and could
	// share one. That is a test-structure change and is recorded rather than
	// smuggled into a refactor PR.
	opts = append(opts, chromedp.WSURLReadTimeout(60*time.Second))
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	return ctx, func() { cancel(); cancelAlloc() }
}

func serveFrontend(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "frontend", "dist")
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		t.Fatalf("frontend missing: %v", err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(root)))
	t.Cleanup(srv.Close)
	return srv.URL
}

func bootApp(t *testing.T, ctx context.Context, url string) {
	t.Helper()
	var ready bool
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(url),
		chromedp.Poll("window.__app && window.__app.ready === true", &ready),
	)
	if err != nil {
		t.Fatalf("app boot: %v", err)
	}
}

func bootAppWithNativeStub(t *testing.T, ctx context.Context, url string, script string) {
	t.Helper()
	var ready bool
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(script).Do(ctx)
			return err
		}),
		chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(url),
		chromedp.Poll("window.__app && window.__app.ready === true", &ready),
	)
	if err != nil {
		t.Fatalf("app boot: %v", err)
	}
}

func evalJS(t *testing.T, ctx context.Context, expr string, out interface{}) {
	t.Helper()
	err := chromedp.Run(ctx, chromedp.Evaluate(expr, out,
		func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}))
	if err != nil {
		t.Fatalf("eval %q: %v", expr, err)
	}
}

// waitForJS polls a boolean JS expression until it holds, and reports whether
// it ever did.
//
// The formatted surface re-renders asynchronously, so any assertion made
// straight after a click or a dispatched change samples a DOM that may still
// be mid-remount. Sampling once makes the test's result depend on machine
// speed: TestContextualDocumentControlsManageBlocksInPlace passed all of
// 2026-08-07 and failed deterministically on 2026-08-08 with nothing in the
// repo changed, catching both the old and new <pre> present and no
// .code-block-shell yet applied.
//
// Use this rather than a bare evalJS whenever the assertion follows an action
// that mutates the document.
func waitForJS(t *testing.T, ctx context.Context, expr string) bool {
	t.Helper()
	var ok bool
	evalJS(t, ctx, `(async () => {
		for (let i = 0; i < 100; i++) {
			if (`+expr+`) return true
			await new Promise((resolve) => setTimeout(resolve, 20))
		}
		return false
	})()`, &ok)
	return ok
}

func TestEditorBoots(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var hasMilkdown bool
	evalJS(t, ctx, "document.querySelector('.milkdown') !== null", &hasMilkdown)
	if !hasMilkdown {
		t.Error("no .milkdown element — Crepe did not mount")
	}

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if md != "" {
		t.Errorf("getMarkdown() = %q, want blank startup document", md)
	}

	var mode string
	evalJS(t, ctx, "window.__app.state.mode", &mode)
	if mode != "wysiwyg" {
		t.Errorf("mode = %q, want wysiwyg", mode)
	}

	var emptyStateVisible bool
	evalJS(t, ctx, "document.body.classList.contains('app-empty') && getComputedStyle(document.getElementById('empty-state')).display !== 'none'", &emptyStateVisible)
	if !emptyStateVisible {
		t.Error("app should launch to the in-canvas empty state")
	}
	// M3.7 Chrome Density removed the in-canvas title bar because the native
	// macOS window owns the application and document NAME. That rationale
	// covers the name and nothing else, so the three things the old blanket
	// assertion lumped together are now asserted separately and for their own
	// reasons — a single rule could not say which of them it meant.
	var duplicatesDocumentTitle bool
	evalJS(t, ctx, `(() => {
		const empty = document.getElementById('empty-state')
		return Array.from(empty.querySelectorAll('h1')).some((node) => /^(Dr\. Markdown|Untitled\.md)$/.test(node.textContent.trim()))
	})()`, &duplicatesDocumentTitle)
	if duplicatesDocumentTitle {
		t.Error("empty state must not repeat the application or document name; the native window title owns it (M3.7)")
	}

	var hasIdentityMark bool
	evalJS(t, ctx, `document.querySelector('#empty-state .empty-logo') !== null`, &hasIdentityMark)
	if !hasIdentityMark {
		t.Error("empty state should carry the app mark: macOS shows no icon in the window title bar, so this is the only place it appears while running")
	}

	var hasShortcutHint bool
	evalJS(t, ctx, `(() => {
		const text = document.getElementById('empty-state').textContent
		return text.includes('⌘⇧R') && text.includes('⌘⇧S')
	})()`, &hasShortcutHint)
	if !hasShortcutHint {
		t.Error("empty state should name the raw and split shortcuts; the native title bar shows no shortcuts, so nothing else discloses them")
	}
	var emptyInspectorHidden bool
	evalJS(t, ctx, `document.body.classList.contains('app-empty') &&
		getComputedStyle(document.getElementById('outline-panel')).display === 'none'`, &emptyInspectorHidden)
	if !emptyInspectorHidden {
		t.Error("empty state should suppress the document inspector until a document exists")
	}

	var contextMenuSuppressed bool
	evalJS(t, ctx, `(() => {
		const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
		document.body.dispatchEvent(event)
		return event.defaultPrevented
	})()`, &contextMenuSuppressed)
	if !contextMenuSuppressed {
		t.Error("browser context menu should be suppressed")
	}

	var fakeTrafficLights int
	evalJS(t, ctx, "document.querySelectorAll('.traffic-lights, .traffic').length", &fakeTrafficLights)
	if fakeTrafficLights != 0 {
		t.Fatalf("fake macOS window controls rendered inside app content: %d", fakeTrafficLights)
	}

	var inCanvasTitleBar bool
	evalJS(t, ctx, "document.getElementById('title-bar') !== null", &inCanvasTitleBar)
	if inCanvasTitleBar {
		t.Fatal("app content should not duplicate the native window title bar")
	}
}

func TestModeToggleRoundTrip(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "# Toggle Test\\n\\nSome **bold** text.\\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown(\""+fixture+"\").then(() => 'ok')", &res)

	// To raw mode: content must match exactly.
	evalJS(t, ctx, "window.__app.toggleMode().then(() => 'ok')", &res)
	var mode string
	evalJS(t, ctx, "window.__app.state.mode", &mode)
	if mode != "raw" {
		t.Fatalf("mode = %q, want raw", mode)
	}
	var rawText string
	evalJS(t, ctx, "window.__app.getMarkdown()", &rawText)
	want := "# Toggle Test\n\nSome **bold** text.\n"
	if rawText != want {
		t.Errorf("raw content = %q, want %q", rawText, want)
	}

	// Edit in raw mode, toggle back: edit must survive.
	evalJS(t, ctx, "window.__app.debugReplaceRaw(\"# Edited\\n\\nNew paragraph.\\n\"); 'ok'", &res)
	evalJS(t, ctx, "window.__app.toggleMode().then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.state.mode", &mode)
	if mode != "wysiwyg" {
		t.Fatalf("mode = %q, want wysiwyg", mode)
	}
	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "# Edited") || !strings.Contains(md, "New paragraph.") {
		t.Errorf("after toggle back, markdown = %q, want the raw edits", md)
	}
}

func TestRawScreenControlsAreBacked(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "# Raw Screen\n\nA very long line that should remain editable while the raw mode soft wrap control changes how the editor lays out the source text without mutating markdown.\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.toggleMode().then(() => 'ok')", &res)

	var rawMode bool
	evalJS(t, ctx, "document.body.classList.contains('raw-mode') && window.__app.state.mode === 'raw'", &rawMode)
	if !rawMode {
		t.Fatal("raw mode should set raw-mode body state")
	}

	var syntaxPanel bool
	evalJS(t, ctx, `document.querySelector('[data-raw-panel="syntax"]') !== null &&
		document.querySelector('[data-raw-toggle="softWrap"]') !== null &&
		document.querySelector('[data-raw-toggle="lineNumbers"]') !== null &&
		document.querySelector('[data-raw-toggle="hideMarkdownMarkers"]') !== null`, &syntaxPanel)
	if !syntaxPanel {
		t.Fatal("raw mode should replace the outline panel with backed syntax/editor controls")
	}

	var markerVisible bool
	evalJS(t, ctx, `document.querySelector('#raw .markdown-marker') !== null &&
		getComputedStyle(document.querySelector('#raw .markdown-marker')).visibility !== 'hidden'`, &markerVisible)
	if !markerVisible {
		t.Fatal("markdown markers should be visible by default in raw mode")
	}
	evalJS(t, ctx, `(() => {
		const toggle = document.querySelector('[data-raw-toggle="hideMarkdownMarkers"]')
		toggle.checked = true
		toggle.dispatchEvent(new Event('change', { bubbles: true }))
		return 'ok'
	})()`, &res)
	var markerHidden bool
	evalJS(t, ctx, `document.body.classList.contains('source-hide-markers') &&
		getComputedStyle(document.querySelector('#raw .markdown-marker')).visibility === 'hidden' &&
		window.__app.getMarkdown() === `+strconv.Quote(fixture), &markerHidden)
	if !markerHidden {
		t.Fatal("Hide markers should hide source marker glyphs without mutating markdown")
	}

	var guttersVisible bool
	evalJS(t, ctx, `getComputedStyle(document.querySelector('#raw .cm-gutters')).display !== 'none'`, &guttersVisible)
	if !guttersVisible {
		t.Fatal("raw line-number gutter should be visible by default")
	}
	evalJS(t, ctx, `(() => {
		const toggle = document.querySelector('[data-raw-toggle="lineNumbers"]')
		toggle.checked = false
		toggle.dispatchEvent(new Event('change', { bubbles: true }))
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, `getComputedStyle(document.querySelector('#raw .cm-gutters')).display === 'none'`, &guttersVisible)
	if !guttersVisible {
		t.Fatal("line-number toggle should hide the raw gutter")
	}

	var wrapActive bool
	evalJS(t, ctx, `document.body.classList.contains('raw-soft-wrap')`, &wrapActive)
	if !wrapActive {
		t.Fatal("soft wrap should be active by default in raw mode")
	}
	evalJS(t, ctx, `(() => {
		const toggle = document.querySelector('[data-raw-toggle="softWrap"]')
		toggle.checked = false
		toggle.dispatchEvent(new Event('change', { bubbles: true }))
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, `document.body.classList.contains('raw-soft-wrap')`, &wrapActive)
	if wrapActive {
		t.Fatal("soft-wrap toggle should update raw layout state")
	}

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if md != fixture {
		t.Fatalf("raw toggles must not mutate markdown: %q", md)
	}
}

func TestSplitSourceHonorsHideMarkdownMarkersWithoutMutatingSource(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "# Split Markers\n\n[Docs](https://example.com)\n\n```go\nfmt.Println(\"ok\")\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setRawOption('hideMarkdownMarkers', true).then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	var hidden bool
	evalJS(t, ctx, `document.body.classList.contains('source-hide-markers') &&
		document.querySelector('#split-source-highlight .markdown-marker') !== null &&
		getComputedStyle(document.querySelector('#split-source-highlight .markdown-marker')).visibility === 'hidden' &&
		document.getElementById('split-source').value === `+strconv.Quote(fixture), &hidden)
	if !hidden {
		t.Fatal("split source should hide markdown marker glyphs without changing textarea source")
	}
}

func TestSplitScreenEditsAndInsertPopoverAreBacked(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "# Split Title\n\nSource paragraph.\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.toggleSplit().then(() => 'ok')", &res)

	var splitVisible bool
	evalJS(t, ctx, `document.body.classList.contains('split-mode') &&
		document.querySelector('[data-split-pane="source"]') !== null &&
		document.querySelector('[data-split-pane="formatted"]') !== null`, &splitVisible)
	if !splitVisible {
		t.Fatal("split mode should show real source and formatted panes")
	}

	evalJS(t, ctx, `(() => {
		const source = document.getElementById('split-source')
		source.value = '# Changed Split\n\nUpdated paragraph.\n'
		source.dispatchEvent(new Event('input', { bubbles: true }))
		return 'ok'
	})()`, &res)

	var previewText, md string
	evalJS(t, ctx, "document.getElementById('split-preview').textContent", &previewText)
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(previewText, "Changed Split") || !strings.Contains(previewText, "Updated paragraph") {
		t.Fatalf("split preview did not refresh from source edit: %q", previewText)
	}
	if !strings.Contains(md, "# Changed Split") {
		t.Fatalf("split source edit did not update markdown: %q", md)
	}

	evalJS(t, ctx, "window.__app.activateRibbonTab('insert'); 'ok'", &res)
	evalJS(t, ctx, "document.getElementById('btn-insert-menu').click(); 'ok'", &res)
	var menuOpen bool
	evalJS(t, ctx, `document.querySelector('[data-insert-menu]') !== null`, &menuOpen)
	if !menuOpen {
		t.Fatal("insert popover should open from the Insert tab")
	}
	evalJS(t, ctx, `document.querySelector('[data-insert-command="hr"]').click(); 'ok'`, &res)
	waitForJS(t, ctx, `window.__app.getMarkdown().includes('---')`)
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "---") {
		t.Fatalf("insert popover command did not mutate markdown: %q", md)
	}

	evalJS(t, ctx, "window.__app.setMode('wysiwyg').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "Changed Split") || (!strings.Contains(md, "---") && !strings.Contains(md, "***")) {
		t.Fatalf("split to formatted round-trip lost markdown: %q", md)
	}
}

func TestSplitModeShowsScrollLockedHeaderAndSyncsScroll(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var builder strings.Builder
	builder.WriteString("# Split Lock\n\n")
	for i := 0; i < 180; i++ {
		builder.WriteString("Paragraph ")
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString(" with enough text to create scrollable split panes.\n\n")
	}

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(builder.String())+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	var header string
	evalJS(t, ctx, `document.querySelector('[data-split-pane="formatted"] .split-header').textContent.trim()`, &header)
	if header != "Formatted · scroll locked" {
		t.Fatalf("formatted split header = %q, want scroll locked label", header)
	}

	var sourceMovesPreview bool
	evalJS(t, ctx, `new Promise((resolve) => {
		const source = document.getElementById('split-source')
		const preview = document.getElementById('split-preview')
		source.scrollTop = source.scrollHeight / 2
		source.dispatchEvent(new Event('scroll', { bubbles: true }))
		setTimeout(() => resolve(preview.scrollTop > 0), 80)
	})`, &sourceMovesPreview)
	if !sourceMovesPreview {
		t.Fatal("scrolling split source should move formatted preview")
	}

	var previewMovesSource bool
	evalJS(t, ctx, `new Promise((resolve) => {
		const source = document.getElementById('split-source')
		const preview = document.getElementById('split-preview')
		source.scrollTop = 0
		preview.scrollTop = preview.scrollHeight / 2
		preview.dispatchEvent(new Event('scroll', { bubbles: true }))
		setTimeout(() => resolve(source.scrollTop > 0), 80)
	})`, &previewMovesSource)
	if !previewMovesSource {
		t.Fatal("scrolling split preview should move source")
	}
}

func TestSourceModesRenderMarkdownSyntaxHighlighting(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "# Highlighted Source\n\nSee [Docs](https://example.com).\n\n```js\nconst answer = 42\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)

	var rawHighlighted bool
	evalJS(t, ctx, `document.querySelector('#raw .source-highlight .hljs-section') !== null &&
		document.querySelector('#raw .source-highlight .hljs-link') !== null`, &rawHighlighted)
	if !rawHighlighted {
		t.Fatal("raw mode should render markdown syntax highlighting")
	}
	var rawText string
	evalJS(t, ctx, "window.__app.getMarkdown()", &rawText)

	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)
	var splitHighlighted bool
	evalJS(t, ctx, `document.querySelector('#split-source-highlight .hljs-section') !== null &&
		document.querySelector('#split-source-highlight .hljs-link') !== null`, &splitHighlighted)
	if !splitHighlighted {
		t.Fatal("split source should render markdown syntax highlighting")
	}

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if md != rawText {
		t.Fatalf("source highlighting must not mutate markdown: %q", md)
	}
}

func TestFencedCodeBlocksUseDeclaredLanguageHighlighting(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "```js\nconst answer = \"yes\"\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	var splitCodeHighlighted bool
	evalJS(t, ctx, `document.querySelector('#split-preview pre code[data-language="js"] .hljs-keyword') !== null &&
		document.querySelector('#split-preview pre code[data-language="js"] .hljs-string') !== null`, &splitCodeHighlighted)
	if !splitCodeHighlighted {
		t.Fatal("split preview fenced code block should highlight using its declared language")
	}

	// The formatted surface highlights through the editor's own code-mirror node
	// view now, not through a Highlight.js pass the app ran over the editor's
	// DOM. So the tokens are CodeMirror's generated highlight classes rather
	// than `.hljs-*`, and the block settles asynchronously: the node view paints
	// a placeholder first and mounts CodeMirror when the block comes into view.
	evalJS(t, ctx, "window.__app.setMode('wysiwyg').then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .cm-content') !== null`) {
		t.Fatal("formatted fenced code block should mount an editable code surface")
	}
	var wysiwygCodeHighlighted bool
	evalJS(t, ctx, `(() => {
		const content = document.querySelector('#wysiwyg .cm-content')
		const language = document.querySelector('#wysiwyg .milkdown-code-block .language-button')
		return content.querySelectorAll('span[class]').length > 0 &&
			language?.textContent.trim().startsWith('js')
	})()`, &wysiwygCodeHighlighted)
	if !wysiwygCodeHighlighted {
		t.Fatal("formatted fenced code block should highlight using its declared language")
	}
}

func TestSplitPreviewRendersInlineMarkdownSemantics(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "This is **strong**, *emphasized*, `inline code`, and a [link](https://example.com).\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	var rendered bool
	evalJS(t, ctx, `(() => {
		const preview = document.getElementById('split-preview')
		return preview.querySelector('strong')?.textContent === 'strong' &&
			preview.querySelector('em')?.textContent === 'emphasized' &&
			preview.querySelector('code:not(pre code)')?.textContent === 'inline code' &&
			preview.querySelector('a[href="https://example.com"]')?.textContent === 'link' &&
			!preview.textContent.includes('[link](https://example.com)')
	})()`, &rendered)
	if !rendered {
		t.Fatal("split preview should render common inline markdown semantics instead of literal markdown")
	}
}

func TestFormattedDocumentUsesConfiguredDocumentFont(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Typography\\n\\nBody text.\\n').then(() => 'ok')", &res)

	var usesDocumentFont bool
	evalJS(t, ctx, `(() => {
		const heading = document.querySelector('#wysiwyg h1')
		const paragraph = document.querySelector('#wysiwyg p')
		const headingFont = getComputedStyle(heading).fontFamily
		const paragraphFont = getComputedStyle(paragraph).fontFamily
		return headingFont.includes('Public Sans') &&
			paragraphFont.includes('Public Sans') &&
			!headingFont.includes('Newsreader') &&
			!paragraphFont.includes('Newsreader')
	})()`, &usesDocumentFont)
	if !usesDocumentFont {
		t.Fatal("formatted document content should use the configured document font, not serif editor defaults")
	}
}

func TestFencedCodeBlocksRenderDesignChrome(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "```javascript\nconst answer = 42\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	var splitChrome bool
	evalJS(t, ctx, `(() => {
		const block = document.querySelector('#split-preview .code-block-shell[data-language="javascript"]')
		return block &&
			block.querySelector('.code-block-language')?.textContent === 'javascript' &&
			block.querySelector('.code-block-copy')?.textContent === 'Copy' &&
			block.querySelector('pre code .hljs-keyword') !== null
	})()`, &splitChrome)
	if !splitChrome {
		t.Fatal("split preview code blocks should render language/copy chrome around highlighted code")
	}

	// In the formatted surface the chrome is the editor's own, not a shell the
	// app injected: a language button that opens a searchable picker, and a copy
	// button, wrapped around a live CodeMirror instead of a static <pre>. The
	// app stopped drawing its own because doing so replaced nodes the node view
	// owns, which is what left code blocks uneditable (#77).
	evalJS(t, ctx, "window.__app.setMode('wysiwyg').then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .copy-button') !== null`) {
		t.Fatal("formatted code blocks should render the editor's language/copy chrome")
	}
	var formattedChrome bool
	evalJS(t, ctx, `(() => {
		const block = document.querySelector('#wysiwyg .milkdown-code-block')
		return block &&
			block.querySelector('.language-button')?.textContent.trim().startsWith('javascript') &&
			block.querySelector('.copy-button') !== null &&
			block.querySelector('.cm-content') !== null
	})()`, &formattedChrome)
	if !formattedChrome {
		t.Fatal("formatted code blocks should render language/copy chrome around an editable code surface")
	}
}

func TestCodeBlockAssistantInsertsSelectedLanguageFromFormattedMode(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Code Assistant\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.activateRibbonTab('insert'); 'ok'", &res)
	evalJS(t, ctx, `document.querySelector('[data-command="code-block"]').click(); 'ok'`, &res)

	var assistantOpen bool
	evalJS(t, ctx, `document.querySelector('[data-code-assistant]') !== null &&
		document.querySelector('[data-code-language]') !== null`, &assistantOpen)
	if !assistantOpen {
		t.Fatal("Code Block insert should open a language assistant")
	}

	evalJS(t, ctx, `(() => {
		const language = document.querySelector('[data-code-language]')
		language.value = 'javascript'
		language.dispatchEvent(new Event('change', { bubbles: true }))
		document.querySelector('[data-code-action="insert"]').click()
		return 'ok'
	})()`, &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "```javascript") {
		t.Fatalf("code assistant did not insert selected language fence: %q", md)
	}

	// An inserted block must arrive ready to type into, which is the whole point
	// of the insert. Before #77 it arrived as an empty grey box that swallowed
	// keystrokes, and no test noticed because none of them typed.
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .cm-content[contenteditable="true"]') !== null`) {
		t.Fatal("inserted code block should be editable in formatted mode")
	}
	var ready bool
	evalJS(t, ctx, `document.querySelector('#wysiwyg .milkdown-code-block .language-button')
		?.textContent.trim().startsWith('javascript') === true`, &ready)
	if !ready {
		t.Fatal("inserted code block should carry the language chosen in the assistant")
	}
}

func TestCodeBlockAssistantCancelDoesNotMutateMarkdown(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# No Code\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.activateRibbonTab('insert'); 'ok'", &res)
	evalJS(t, ctx, `document.querySelector('[data-command="code-block"]').click(); 'ok'`, &res)
	evalJS(t, ctx, `document.querySelector('[data-code-action="cancel"]').click(); 'ok'`, &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if strings.Contains(md, "```") {
		t.Fatalf("canceling code assistant should not insert markdown: %q", md)
	}
}

func TestMermaidDiagramAssistantInsertsSelectedDiagramType(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Diagram\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.activateRibbonTab('insert'); 'ok'", &res)
	evalJS(t, ctx, "document.getElementById('btn-insert-menu').click(); 'ok'", &res)
	evalJS(t, ctx, `document.querySelector('[data-insert-command="mermaid"]').click(); 'ok'`, &res)

	var assistantOpen bool
	evalJS(t, ctx, `document.querySelector('[data-diagram-assistant]') !== null &&
		document.querySelector('[data-diagram-type="sequence"]') !== null`, &assistantOpen)
	if !assistantOpen {
		t.Fatal("Mermaid Diagram insert should open a diagram type assistant")
	}

	evalJS(t, ctx, `document.querySelector('[data-diagram-type="sequence"]').click(); 'ok'`, &res)
	var guidedFields bool
	evalJS(t, ctx, `document.querySelector('[data-diagram-field="participantA"]') !== null &&
		document.querySelector('[data-diagram-field="participantB"]') !== null &&
		document.querySelector('[data-diagram-preview] svg') !== null`, &guidedFields)
	if !guidedFields {
		t.Fatal("diagram assistant should expose type-specific fields and a live rendered preview")
	}
	evalJS(t, ctx, `(() => {
		const participant = document.querySelector('[data-diagram-field="participantA"]')
		participant.value = 'Writer'
		participant.dispatchEvent(new Event('input', { bubbles: true }))
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, `document.querySelector('[data-diagram-action="insert"]').click(); 'ok'`, &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "```mermaid") || !strings.Contains(md, "sequenceDiagram") || !strings.Contains(md, "Writer") {
		t.Fatalf("diagram assistant did not insert the selected Mermaid starter: %q", md)
	}
}

func TestMermaidDiagramAssistantCancelDoesNotMutateMarkdown(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# No Diagram\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "document.querySelector('[data-command=\"mermaid\"]').click(); 'ok'", &res)
	evalJS(t, ctx, `document.querySelector('[data-diagram-action="cancel"]').click(); 'ok'`, &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if strings.Contains(md, "```mermaid") {
		t.Fatalf("canceling diagram assistant should not insert markdown: %q", md)
	}
}

func TestMermaidBlocksRenderAsDiagrams(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "```mermaid\ngraph TD\n  A[Start] --> B[Finish]\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)

	var splitRendered bool
	evalJS(t, ctx, `document.querySelector('#split-preview .mermaid-render svg') !== null &&
		document.querySelector('#split-preview pre code[data-language="mermaid"]') === null`, &splitRendered)
	if !splitRendered {
		t.Fatal("split preview should render Mermaid diagrams instead of showing a mermaid code block")
	}

	// The diagram is drawn by the editor's code-block preview hook, which runs
	// when the node view mounts CodeMirror — one frame after the surface itself,
	// because that mounting is driven by an IntersectionObserver. The diagram's
	// SVG is primed before the editor builds, so it appears complete rather than
	// popping in, but the block it lives in still arrives asynchronously.
	evalJS(t, ctx, "window.__app.setMode('wysiwyg').then(() => 'ok')", &res)
	if !waitForJS(t, ctx, `document.querySelector('#wysiwyg .mermaid-render svg') !== null`) {
		t.Fatal("formatted mode should render Mermaid diagrams")
	}
}

func TestFileFlowWithStubBridge(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	stub := `globalThis.__calls = [];
globalThis.drmd = { native: {
  OpenDocument: async () => ({ path: '/tmp/fake.md', content: "# From Stub\n" }),
  SaveDocument: async (p, c) => { globalThis.__calls.push(['save', p, c]); },
  SaveDocumentAs: async (c) => { globalThis.__calls.push(['saveAs', c]); return '/tmp/fake.md'; },
  SetDirty: (d) => { globalThis.__calls.push(['dirty', d]); },
  UpdateContent: (c) => { globalThis.__calls.push(['content', c]); },
} } ; 'ok'`
	var res string
	evalJS(t, ctx, stub, &res)

	// Open: editor shows stub content, path recorded.
	evalJS(t, ctx, "window.__app.openDocument().then(() => 'ok')", &res)
	var md, path string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	evalJS(t, ctx, "window.__app.state.path", &path)
	if !strings.Contains(md, "# From Stub") {
		t.Errorf("after open, markdown = %q", md)
	}
	if path != "/tmp/fake.md" {
		t.Errorf("path = %q, want /tmp/fake.md", path)
	}
	var emptyStateStillVisible bool
	evalJS(t, ctx, `getComputedStyle(document.getElementById('empty-state')).display !== 'none'`, &emptyStateStillVisible)
	if emptyStateStillVisible {
		t.Error("empty state should be hidden after opening a document")
	}

	// Save: routes to SaveDocument with the current path and content.
	evalJS(t, ctx, "window.__app.save().then(() => 'ok')", &res)
	var savedPath, savedContent string
	evalJS(t, ctx, "globalThis.__calls.filter(c => c[0] === 'save')[0][1]", &savedPath)
	evalJS(t, ctx, "globalThis.__calls.filter(c => c[0] === 'save')[0][2]", &savedContent)
	if savedPath != "/tmp/fake.md" {
		t.Errorf("save path = %q", savedPath)
	}
	if !strings.Contains(savedContent, "# From Stub") {
		t.Errorf("save content = %q", savedContent)
	}
	var dirty bool
	evalJS(t, ctx, "window.__app.state.dirty", &dirty)
	if dirty {
		t.Error("dirty should be false after save")
	}
}

func TestDirtyWiringWithStubBridge(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	stub := `globalThis.__calls = [];
globalThis.drmd = { native: {
  SetDirty: (d) => { globalThis.__calls.push(['dirty', d]); },
  UpdateContent: (c) => { globalThis.__calls.push(['content', c]); },
} } ; 'ok'`
	var res string
	evalJS(t, ctx, stub, &res)

	evalJS(t, ctx, "window.__app.debugSimulateEdit('# Changed\\n'); 'ok'", &res)

	var dirty bool
	evalJS(t, ctx, "window.__app.state.dirty", &dirty)
	if !dirty {
		t.Error("state.dirty should be true after edit")
	}

	// SetDirty is pushed immediately (not debounced): it must already be
	// at the bridge, while UpdateContent is still pending its 300ms flush.
	var dirtyNow, contentNow bool
	evalJS(t, ctx, "globalThis.__calls.some(c => c[0] === 'dirty' && c[1] === true)", &dirtyNow)
	evalJS(t, ctx, "globalThis.__calls.some(c => c[0] === 'content')", &contentNow)
	if !dirtyNow {
		t.Error("bridge should have received SetDirty(true) immediately")
	}
	if contentNow {
		t.Error("UpdateContent should still be debounced right after the edit")
	}

	// Debounced push: wait for it, then assert UpdateContent reached the
	// bridge too.
	var pushed bool
	evalJS(t, ctx, `new Promise(r => setTimeout(() => {
		r(globalThis.__calls.some(c => c[0] === 'content' && c[1].includes('# Changed')))
	}, 600))`, &pushed)
	if !pushed {
		t.Errorf("bridge did not receive debounced UpdateContent; calls were pushed via debugSimulateEdit")
	}
}

func TestRawModeDirtyTracking(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	stub := `globalThis.__calls = [];
globalThis.drmd = { native: {
  SetDirty: (d) => { globalThis.__calls.push(['dirty', d]); },
  UpdateContent: (c) => { globalThis.__calls.push(['content', c]); },
} } ; 'ok'`
	var res string
	evalJS(t, ctx, stub, &res)

	// Switch to raw mode, then edit through CodeMirror's dispatch path
	// (same as typing): the updateListener must feed dirty tracking.
	evalJS(t, ctx, "window.__app.toggleMode().then(() => 'ok')", &res)
	var mode string
	evalJS(t, ctx, "window.__app.state.mode", &mode)
	if mode != "raw" {
		t.Fatalf("mode = %q, want raw", mode)
	}
	evalJS(t, ctx, "window.__app.debugReplaceRaw('# Changed\\n'); 'ok'", &res)

	var dirty bool
	evalJS(t, ctx, "window.__app.state.dirty", &dirty)
	if !dirty {
		t.Error("state.dirty should be true after a raw-mode edit")
	}

	// Immediate SetDirty(true) plus debounced UpdateContent with the new text.
	var pushed bool
	evalJS(t, ctx, `new Promise(r => setTimeout(() => {
		r(globalThis.__calls.some(c => c[0] === 'dirty' && c[1] === true) &&
		  globalThis.__calls.some(c => c[0] === 'content' && c[1].includes('# Changed')))
	}, 600))`, &pushed)
	if !pushed {
		t.Errorf("raw-mode edit did not reach the bridge; calls = see page state")
	}
}

func TestOpenOverDirtyGuard(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	stub := `globalThis.__calls = [];
globalThis.__resolveResult = false;
globalThis.drmd = { native: {
  OpenDocument: async () => {
    globalThis.__calls.push(['open']);
    return { path: '/tmp/guard.md', content: "# Opened Doc\n" };
  },
  ResolveUnsavedChanges: async () => {
    globalThis.__calls.push(['resolve']);
    return globalThis.__resolveResult;
  },
  SetDirty: (d) => { globalThis.__calls.push(['dirty', d]); },
  UpdateContent: (c) => { globalThis.__calls.push(['content', c]); },
} } ; 'ok'`
	var res string
	evalJS(t, ctx, stub, &res)

	// Dirty the buffer, then try to open while the guard says "Cancel":
	// the open must be aborted before OpenDocument is even called.
	evalJS(t, ctx, "window.__app.debugSimulateEdit('# Dirty Edit\\n'); 'ok'", &res)
	var before string
	evalJS(t, ctx, "window.__app.getMarkdown()", &before)
	evalJS(t, ctx, "window.__app.openDocument().then(() => 'ok')", &res)

	var after string
	evalJS(t, ctx, "window.__app.getMarkdown()", &after)
	if after != before {
		t.Errorf("open was not aborted: content changed to %q", after)
	}
	var openCalls int
	evalJS(t, ctx, "globalThis.__calls.filter(c => c[0] === 'open').length", &openCalls)
	if openCalls != 0 {
		t.Errorf("OpenDocument called %d times despite guard", openCalls)
	}
	var resolveCalls int
	evalJS(t, ctx, "globalThis.__calls.filter(c => c[0] === 'resolve').length", &resolveCalls)
	if resolveCalls != 1 {
		t.Errorf("ResolveUnsavedChanges called %d times, want 1", resolveCalls)
	}

	// Guard says "proceed": the open must go through.
	evalJS(t, ctx, "globalThis.__resolveResult = true; 'ok'", &res)
	evalJS(t, ctx, "window.__app.openDocument().then(() => 'ok')", &res)
	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "# Opened Doc") {
		t.Errorf("open did not proceed; markdown = %q", md)
	}
	evalJS(t, ctx, "globalThis.__calls.filter(c => c[0] === 'open').length", &openCalls)
	if openCalls != 1 {
		t.Errorf("OpenDocument called %d times, want 1", openCalls)
	}
	var dirty bool
	evalJS(t, ctx, "window.__app.state.dirty", &dirty)
	if dirty {
		t.Error("dirty should be false after the guarded open went through")
	}
}

func TestRibbonCommandsInsertMarkdown(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Ribbon\\n\\nBody\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.runCommand('bold').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.runCommand('table').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.runCommand('mermaid').then(() => 'ok')", &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	for _, want := range []string{"**bold text**", "| Header 1 | Header 2 | Header 3 |", "```mermaid"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	var tableToolsPresent bool
	evalJS(t, ctx, "document.getElementById('tab-table') !== null", &tableToolsPresent)
	if tableToolsPresent {
		t.Error("table mutation tools should not live in the ribbon")
	}
}

func TestRibbonFormattingUsesSelectionInsteadOfAppending(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('Title\\n\\nBody text\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => {
		const walker = document.createTreeWalker(document.getElementById('wysiwyg'), NodeFilter.SHOW_TEXT)
		let node = walker.nextNode()
		while (node && !node.nodeValue.includes('Title')) node = walker.nextNode()
		const range = document.createRange()
		range.setStart(node, node.nodeValue.indexOf('Title'))
		range.setEnd(node, node.nodeValue.indexOf('Title') + 'Title'.length)
		const selection = window.getSelection()
		selection.removeAllRanges()
		selection.addRange(range)
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.runCommand('h2').then(() => 'ok')", &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "## Title") {
		t.Fatalf("H2 did not format the selected text: %q", md)
	}
	if strings.Contains(md, "## Heading 2") {
		t.Fatalf("H2 appended a new placeholder heading instead of formatting selection: %q", md)
	}

	evalJS(t, ctx, "window.__app.setMarkdown('Highlight this text\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => {
		const walker = document.createTreeWalker(document.getElementById('wysiwyg'), NodeFilter.SHOW_TEXT)
		let node = walker.nextNode()
		while (node && !node.nodeValue.includes('this')) node = walker.nextNode()
		const range = document.createRange()
		range.setStart(node, node.nodeValue.indexOf('this'))
		range.setEnd(node, node.nodeValue.indexOf('this') + 'this'.length)
		const selection = window.getSelection()
		selection.removeAllRanges()
		selection.addRange(range)
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.runCommand('highlight').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "Highlight <mark>this</mark> text") {
		t.Fatalf("highlight did not format the selected text: %q", md)
	}
	if strings.Contains(md, "<mark>highlighted text</mark>") {
		t.Fatalf("highlight appended placeholder text instead of formatting selection: %q", md)
	}
}

func TestStructureControlsFormatCurrentBlockInsteadOfAppending(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('Cursor title\\n\\nBody text\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => {
		const walker = document.createTreeWalker(document.getElementById('wysiwyg'), NodeFilter.SHOW_TEXT)
		let node = walker.nextNode()
		while (node && !node.nodeValue.includes('Cursor title')) node = walker.nextNode()
		const range = document.createRange()
		range.setStart(node, 'Cursor'.length)
		range.collapse(true)
		const selection = window.getSelection()
		selection.removeAllRanges()
		selection.addRange(range)
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.runCommand('h2').then(() => 'ok')", &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "## Cursor title") {
		t.Fatalf("H2 did not format the current paragraph: %q", md)
	}
	if strings.Contains(md, "## Heading 2") {
		t.Fatalf("H2 appended a placeholder heading from a cursor position: %q", md)
	}

	evalJS(t, ctx, "window.__app.setMarkdown('Dropdown title\\n\\nBody text\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => {
		const walker = document.createTreeWalker(document.getElementById('wysiwyg'), NodeFilter.SHOW_TEXT)
		let node = walker.nextNode()
		while (node && !node.nodeValue.includes('Dropdown title')) node = walker.nextNode()
		const range = document.createRange()
		range.setStart(node, node.nodeValue.indexOf('Dropdown title'))
		range.setEnd(node, node.nodeValue.indexOf('Dropdown title') + 'Dropdown title'.length)
		const selection = window.getSelection()
		selection.removeAllRanges()
		selection.addRange(range)
		const select = document.getElementById('block-style')
		select.value = 'h3'
		select.dispatchEvent(new Event('change', { bubbles: true }))
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "### Dropdown title") {
		t.Fatalf("paragraph style dropdown did not format selected text: %q", md)
	}
	if strings.Contains(md, "### Heading 3") {
		t.Fatalf("paragraph style dropdown appended a placeholder heading: %q", md)
	}
}

func TestBlockCommandsFormatCurrentBlockInsteadOfAppending(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('Todo item\\n\\nNext paragraph\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => {
		const walker = document.createTreeWalker(document.getElementById('wysiwyg'), NodeFilter.SHOW_TEXT)
		let node = walker.nextNode()
		while (node && !node.nodeValue.includes('Todo item')) node = walker.nextNode()
		const range = document.createRange()
		range.setStart(node, 'Todo'.length)
		range.collapse(true)
		const selection = window.getSelection()
		selection.removeAllRanges()
		selection.addRange(range)
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.runCommand('task-list').then(() => 'ok')", &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "[ ] Todo item") {
		t.Fatalf("task list did not format the current paragraph: %q", md)
	}
	if strings.Contains(md, "- [ ] Task item") {
		t.Fatalf("task list appended a placeholder item: %q", md)
	}

	evalJS(t, ctx, "window.__app.setMarkdown('code line\\n\\nNext paragraph\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => {
		const walker = document.createTreeWalker(document.getElementById('wysiwyg'), NodeFilter.SHOW_TEXT)
		let node = walker.nextNode()
		while (node && !node.nodeValue.includes('code line')) node = walker.nextNode()
		const range = document.createRange()
		range.setStart(node, 0)
		range.setEnd(node, node.nodeValue.length)
		const selection = window.getSelection()
		selection.removeAllRanges()
		selection.addRange(range)
		const select = document.getElementById('block-style')
		select.value = 'code-block'
		select.dispatchEvent(new Event('change', { bubbles: true }))
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "```text\ncode line\n```") {
		t.Fatalf("code block style did not format selected/current text: %q", md)
	}
	if strings.Contains(md, "```text\ncode\n```") {
		t.Fatalf("code block style appended placeholder code: %q", md)
	}
}

func TestRibbonTabSwitcherShowsBackedPanels(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Ribbon Tabs\\n').then(() => 'ok')", &res)

	for _, tc := range []struct {
		tab     string
		command string
	}{
		{tab: "format", command: "bold"},
		{tab: "insert", command: "table"},
		{tab: "view", command: "theme"},
	} {
		evalJS(t, ctx, "window.__app.activateRibbonTab('"+tc.tab+"'); 'ok'", &res)
		var visible bool
		evalJS(t, ctx,
			"document.querySelector('[data-ribbon-panel=\""+tc.tab+"\"]').hidden === false && "+
				"document.querySelector('[data-ribbon-panel=\""+tc.tab+"\"] [data-command=\""+tc.command+"\"]') !== null",
			&visible)
		if !visible {
			t.Errorf("ribbon tab %q did not expose backed command %q", tc.tab, tc.command)
		}
	}
}

func TestRibbonCompletionCommandsAreBacked(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	// Image insertion is backed by the native import bridge, so the ribbon
	// wiring can only be exercised with that binding stubbed.
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		ImportImage: async () => ({ markdown: '![shot](doc.assets/shot.png)' }),
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; 'ok'`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown('# Ribbon Completion\\n').then(() => 'ok')", &res)

	// The insert commands live in the Insert tab alone now. They used to be in
	// Home as well, which is why Home was removed: it was a superset of Insert
	// and Format, so two of five tabs taught the reader nothing new.
	var insertCommands bool
	evalJS(t, ctx, `['link','image','table','code-block','math','mermaid'].every((command) =>
		document.querySelector('[data-ribbon-panel="insert"] [data-command="' + command + '"]'))`, &insertCommands)
	if !insertCommands {
		t.Fatal("the Insert ribbon should expose the backed Insert command set")
	}

	evalJS(t, ctx, "window.__app.activateRibbonTab('insert'); 'ok'", &res)
	// The image command round-trips through the native import bridge, so wait
	// for the insertion to settle instead of reading markdown synchronously.
	evalJS(t, ctx, `(async () => {
		document.querySelector('[data-command="image"]').click()
		for (let i = 0; i < 100 && !window.__app.getMarkdown().includes('doc.assets/shot.png'); i++) {
			await new Promise((resolve) => setTimeout(resolve, 20))
		}
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, `document.querySelector('[data-command="math"]').click(); 'ok'`, &res)
	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "doc.assets/shot.png") || !strings.Contains(md, "$$") {
		t.Fatalf("Image and Math commands should insert backed markdown, got %q", md)
	}

	evalJS(t, ctx, "window.__app.activateRibbonTab('help'); 'ok'", &res)
	evalJS(t, ctx, `document.querySelector('[data-help-toggle]').click(); 'ok'`, &res)
	var helpOpen bool
	evalJS(t, ctx, `document.querySelector('[data-help-panel]') !== null`, &helpOpen)
	if !helpOpen {
		t.Fatal("Help tab should open a backed help panel")
	}

	var shareRendered bool
	evalJS(t, ctx, `Array.from(document.querySelectorAll('button')).some((button) => button.textContent.trim() === 'Share')`, &shareRendered)
	if shareRendered {
		t.Fatal("Share should remain hidden until collaboration behavior is backed")
	}

	// Print and PDF moved out of a ribbon dropdown and into the File menu when
	// File stopped being a tab. The guarantee is unchanged: both must be present
	// and backed, not merely drawn.
	var exportOpen bool
	evalJS(t, ctx, `(async () => {
		document.getElementById('btn-file-menu').click()
		await new Promise((r) => setTimeout(r, 250))
		return document.querySelector('[data-file-menu]') !== null &&
			document.querySelector('[data-file-menu-action="print"]') !== null &&
			document.querySelector('[data-file-menu-action="pdf"]') !== null
	})()`, &exportOpen)
	if !exportOpen {
		t.Fatal("the File menu should offer backed Print and PDF actions")
	}
}

func TestCurrentScreenHasNoDecorativeEnabledControls(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var decorative []string
	evalJS(t, ctx, `Array.from(document.querySelectorAll('#title-bar button:not([disabled]), #ribbon button:not([disabled]), #file-rail button:not([disabled]), #empty-state button:not([disabled]), #outline-panel button:not([disabled])'))
		.filter((button) => {
			const backed =
				button.dataset.command ||
				button.dataset.ribbonTab ||
				button.dataset.template ||
				button.dataset.outlineTab ||
				button.dataset.panelToggle ||
				button.dataset.exportToggle ||
				// data-file-action is how New/Open/Save/Save As are wired: by
				// attribute rather than by id, because they now appear in the File
				// tab, the rail and the empty state, and an id names one element.
				button.dataset.fileAction ||
				button.dataset.helpToggle ||
				button.dataset.shortcutsToggle ||
				button.id ||
				button.closest('#file-list')
			return !backed
		})
		.map((button) => button.textContent.trim())`, &decorative)
	if len(decorative) != 0 {
		t.Fatalf("enabled decorative buttons rendered: %v", decorative)
	}

	var futureLabels int
	evalJS(t, ctx, `Array.from(document.querySelectorAll('button'))
		.filter((button) => /Preview|Share/.test(button.textContent))
		.length`, &futureLabels)
	if futureLabels != 0 {
		t.Fatalf("future-only controls should not render on the functional screen: %d", futureLabels)
	}
}

func TestImageRibbonCommandImportsNativeAssetMarkdown(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.__imageImportPath = '';
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		ImportImage: async (documentPath) => {
			globalThis.__imageImportPath = documentPath
			return { markdown: '![photo](doc.assets/photo.png)', markdownPath: 'doc.assets/photo.png' }
		},
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; 'ok'`, &res)
	evalJS(t, ctx, `(() => {
		const doc = window.__app.state.docs.find((item) => item.id === window.__app.state.activeDocId)
		doc.path = '/tmp/doc.md'
		doc.title = 'doc.md'
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.runCommand('image').then(() => 'ok')", &res)

	var imported bool
	evalJS(t, ctx, `globalThis.__imageImportPath === '/tmp/doc.md' &&
		window.__app.getMarkdown().includes('![photo](doc.assets/photo.png)') &&
		!window.__app.getMarkdown().includes('image-placeholder.png')`, &imported)
	if !imported {
		t.Fatal("Image command should insert native imported asset markdown, not a placeholder")
	}
}

// A rejected import is already reported by the native error dialog, so the
// frontend must leave the document untouched rather than inserting a
// non-portable placeholder or leaving the command chain in a rejected state.
func TestImageRibbonCommandLeavesDocumentUnchangedWhenImportFails(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		ImportImage: async () => { throw new Error('Save the document before inserting images.') },
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; 'ok'`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown('# Import Failure\\n').then(() => 'ok')", &res)

	var settled string
	evalJS(t, ctx,
		"window.__app.runCommand('image').then(() => 'resolved').catch(() => 'rejected')",
		&settled)
	if settled != "resolved" {
		t.Fatalf("failed import should settle the command chain, got %q", settled)
	}

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if strings.Contains(md, "![") {
		t.Fatalf("failed import should not insert image markdown, got %q", md)
	}
}

// Relative asset paths cannot resolve against the webview origin, so the
// editor must inline them through the bridge or imported images never appear.
func TestRenderedImagesResolveThroughAssetBridge(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.__loadedAssets = [];
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async (documentPath, markdownPath) => {
			globalThis.__loadedAssets.push([documentPath, markdownPath])
			return { dataURI: 'data:image/png;base64,AAAA', exists: true, absolutePath: '/tmp/doc.assets/photo.png' }
		},
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; 'ok'`, &res)
	evalJS(t, ctx, `(() => {
		const doc = window.__app.state.docs.find((item) => item.id === window.__app.state.activeDocId)
		doc.path = '/tmp/doc.md'
		return 'ok'
	})()`, &res)
	evalJS(t, ctx,
		"window.__app.setMarkdown('# Shot\\n\\n![photo](doc.assets/photo.png)\\n').then(() => 'ok')", &res)

	var resolved bool
	evalJS(t, ctx, `(async () => {
		for (let i = 0; i < 100; i++) {
			const img = document.querySelector('#wysiwyg img')
			if (img && img.src.startsWith('data:image/png;base64,AAAA')) return true
			await new Promise((resolve) => setTimeout(resolve, 20))
		}
		return false
	})()`, &resolved)
	if !resolved {
		t.Fatal("relative image should be inlined from the asset bridge")
	}

	var requested string
	evalJS(t, ctx, "JSON.stringify(globalThis.__loadedAssets[0] || [])", &requested)
	if !strings.Contains(requested, "/tmp/doc.md") || !strings.Contains(requested, "doc.assets/photo.png") {
		t.Fatalf("asset request = %s, want document path and markdown path", requested)
	}
}

// A deleted or moved asset must be visibly broken rather than silently blank,
// so the user can tell the difference between "no image" and "lost image".
func TestMissingImageAssetRendersVisibleBrokenState(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: '', exists: false, absolutePath: '/tmp/doc.assets/gone.png' }),
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; 'ok'`, &res)
	evalJS(t, ctx, `(() => {
		const doc = window.__app.state.docs.find((item) => item.id === window.__app.state.activeDocId)
		doc.path = '/tmp/doc.md'
		return 'ok'
	})()`, &res)
	evalJS(t, ctx,
		"window.__app.setMarkdown('![gone](doc.assets/gone.png)\\n').then(() => 'ok')", &res)

	var broken bool
	evalJS(t, ctx, `(async () => {
		for (let i = 0; i < 100; i++) {
			const img = document.querySelector('#wysiwyg img[data-missing-asset]')
			if (img) return true
			await new Promise((resolve) => setTimeout(resolve, 20))
		}
		return false
	})()`, &broken)
	if !broken {
		t.Fatal("missing asset should be marked with data-missing-asset for a visible broken state")
	}
}

// Print and PDF export render from the preview pipeline, so images must be
// inlined there too or exported artifacts lose every local image.
func TestPrintExportInlinesImageAssets(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: 'data:image/png;base64,PRINTED', exists: true }),
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; window.print = () => { globalThis.__printed = true }; 'ok'`, &res)
	evalJS(t, ctx, `(() => {
		const doc = window.__app.state.docs.find((item) => item.id === window.__app.state.activeDocId)
		doc.path = '/tmp/doc.md'
		return 'ok'
	})()`, &res)
	evalJS(t, ctx,
		"window.__app.setMarkdown('![photo](doc.assets/photo.png)\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.printDocument('pdf').then(() => 'ok')", &res)

	var inlined bool
	evalJS(t, ctx,
		`Boolean(document.querySelector('#print-root img[src^="data:image/png;base64,PRINTED"]'))`,
		&inlined)
	if !inlined {
		t.Fatal("print/export render should inline image assets as data URIs")
	}
}

// An opened document is untrusted input rendered in a webview that holds the
// native file bindings, so inline <img> tags must not carry event handlers
// through to the DOM.
func TestPreviewDropsEventHandlersFromDocumentImageTags(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: '', exists: false }),
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; window.print = () => {}; 'ok'`, &res)
	evalJS(t, ctx, `window.__app.setMarkdown('<img src="x.png" alt="a" width="100" onerror="globalThis.__pwned = true">\n').then(() => 'ok')`, &res)
	evalJS(t, ctx, "window.__app.printDocument('print').then(() => 'ok')", &res)

	var handlerCarried bool
	evalJS(t, ctx,
		`Array.from(document.querySelectorAll('#print-root img')).some((img) => img.hasAttribute('onerror'))`,
		&handlerCarried)
	if handlerCarried {
		t.Fatal("document-supplied image tags must not carry event handlers into the DOM")
	}

	var widthKept string
	evalJS(t, ctx,
		`document.querySelector('#print-root img')?.getAttribute('width') ?? ''`, &widthKept)
	if widthKept != "100" {
		t.Fatalf("supported width attribute should survive sanitising, got %q", widthKept)
	}
}

const twoImageDocument = "# Gallery\n\n![first](doc.assets/first.png)\n\n![second](doc.assets/second.png)\n"

// bootImageDocument loads a two-image document with the asset bridge stubbed
// and selects the image at the given index as the acted-on block.
func bootImageDocument(t *testing.T, ctx context.Context, selectIndex int) {
	t.Helper()
	var res string
	evalJS(t, ctx, `globalThis.__revealed = null;
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: 'data:image/png;base64,AAAA', exists: true }),
		RevealImageAsset: async (documentPath, markdownPath) => { globalThis.__revealed = [documentPath, markdownPath] },
		ImportImage: async () => ({ markdown: '![swapped](doc.assets/swapped.png)', markdownPath: 'doc.assets/swapped.png' }),
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; 'ok'`, &res)
	evalJS(t, ctx, `(() => {
		const doc = window.__app.state.docs.find((item) => item.id === window.__app.state.activeDocId)
		doc.path = '/tmp/doc.md'
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(twoImageDocument)+").then(() => 'ok')", &res)
	evalJS(t, ctx, `(async () => {
		for (let i = 0; i < 100; i++) {
			const images = document.querySelectorAll('#wysiwyg img')
			if (images.length >= 2) { images[`+strconv.Itoa(selectIndex)+`].click(); return 'ok' }
			await new Promise((resolve) => setTimeout(resolve, 20))
		}
		return 'timeout'
	})()`, &res)
	if res != "ok" {
		t.Fatalf("image document did not render two images: %q", res)
	}
}

// Contextual controls must act on the selected image, not the first one found.
func TestImageControlsActOnSelectedImageOnly(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)
	bootImageDocument(t, ctx, 1)

	var res string
	evalJS(t, ctx, "window.__app.setImageAltText('renamed').then(() => 'ok')", &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "![first](doc.assets/first.png)") {
		t.Errorf("unselected image must not change, got %q", md)
	}
	if !strings.Contains(md, "![renamed](doc.assets/second.png)") {
		t.Errorf("selected image alt text should change, got %q", md)
	}
}

// Sizing is expressed as an <img> tag because CommonMark has no size syntax;
// clearing the width must return the image to portable markdown.
func TestImageWidthRoundTripsBetweenMarkdownAndHTMLForms(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)
	bootImageDocument(t, ctx, 0)

	var res string
	evalJS(t, ctx, "window.__app.setImageWidth('400').then(() => 'ok')", &res)
	var sized string
	evalJS(t, ctx, "window.__app.getMarkdown()", &sized)
	if !strings.Contains(sized, `<img src="doc.assets/first.png" alt="first" width="400">`) {
		t.Fatalf("sized image should become an img tag, got %q", sized)
	}
	if !strings.Contains(sized, "![second](doc.assets/second.png)") {
		t.Errorf("sibling image must be untouched, got %q", sized)
	}

	evalJS(t, ctx, "window.__app.setImageWidth('').then(() => 'ok')", &res)
	var restored string
	evalJS(t, ctx, "window.__app.getMarkdown()", &restored)
	if !strings.Contains(restored, "![first](doc.assets/first.png)") {
		t.Fatalf("clearing width should restore portable markdown, got %q", restored)
	}
}

func TestImageDeleteRemovesOnlySelectedImage(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)
	bootImageDocument(t, ctx, 0)

	var res string
	evalJS(t, ctx, "window.__app.runCommand('image-delete').then(() => 'ok')", &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if strings.Contains(md, "first.png") {
		t.Errorf("selected image should be removed, got %q", md)
	}
	if !strings.Contains(md, "![second](doc.assets/second.png)") {
		t.Errorf("sibling image should survive, got %q", md)
	}
}

func TestImageReplaceSwapsSelectedImageSource(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)
	bootImageDocument(t, ctx, 1)

	var res string
	evalJS(t, ctx, "window.__app.runCommand('image-replace').then(() => 'ok')", &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "doc.assets/swapped.png") {
		t.Errorf("selected image source should be replaced, got %q", md)
	}
	if !strings.Contains(md, "![first](doc.assets/first.png)") {
		t.Errorf("unselected image must not be replaced, got %q", md)
	}
	if strings.Contains(md, "second.png") {
		t.Errorf("replaced source should not remain, got %q", md)
	}
}

func TestImageRevealRoutesSelectedAssetToNativeReveal(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)
	bootImageDocument(t, ctx, 1)

	var res string
	evalJS(t, ctx, "window.__app.runCommand('image-reveal').then(() => 'ok')", &res)

	var revealed string
	evalJS(t, ctx, "JSON.stringify(globalThis.__revealed || [])", &revealed)
	if !strings.Contains(revealed, "doc.assets/second.png") {
		t.Fatalf("reveal should target the selected asset, got %s", revealed)
	}
}

// Dropped image files import through the same asset policy as the ribbon
// command; non-image files are ignored rather than copied into the assets dir.
func TestDroppedImageFilesImportThroughAssetPolicy(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.__dropped = [];
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: 'data:image/png;base64,AAAA', exists: true }),
		ImportDroppedImage: async (documentPath, sourcePath) => {
			globalThis.__dropped.push(sourcePath)
			return { markdown: '![shot](doc.assets/shot.png)', markdownPath: 'doc.assets/shot.png' }
		},
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; 'ok'`, &res)
	evalJS(t, ctx, `(() => {
		const doc = window.__app.state.docs.find((item) => item.id === window.__app.state.activeDocId)
		doc.path = '/tmp/doc.md'
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown('# Drop\\n').then(() => 'ok')", &res)
	evalJS(t, ctx,
		`window.__app.handleDroppedFiles(['/tmp/notes.txt', '/tmp/shot.png']).then(() => 'ok')`, &res)

	var dropped string
	evalJS(t, ctx, "JSON.stringify(globalThis.__dropped)", &dropped)
	if strings.Contains(dropped, "notes.txt") {
		t.Errorf("non-image drops should be ignored, got %s", dropped)
	}
	if !strings.Contains(dropped, "/tmp/shot.png") {
		t.Fatalf("dropped image should import, got %s", dropped)
	}

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "![shot](doc.assets/shot.png)") {
		t.Fatalf("dropped image markdown should be inserted, got %q", md)
	}
}

// The editor is the product; a bridge call failing at boot must not stop it
// mounting. Previously a corrupt preferences.json rejected here and left a
// blank window with no in-app recovery (issue #17).
func TestBootSurvivesRejectedPreferences(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)

	bootAppWithNativeStub(t, ctx, url, `globalThis.drmd = { native: {
		LoadPreferences: async () => { throw new Error('decode preferences: unexpected end of JSON input') },
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ;`)

	var mounted bool
	evalJS(t, ctx, `Boolean(window.__app && window.__app.ready === true &&
		document.getElementById('wysiwyg') !== null)`, &mounted)
	if !mounted {
		t.Fatal("a rejected preferences load must not prevent the editor mounting")
	}

	// Defaults must still apply rather than leaving the shell unstyled.
	var settingsApplied bool
	evalJS(t, ctx, `typeof window.__app.state.settings.documentFontSize === 'number'`, &settingsApplied)
	if !settingsApplied {
		t.Error("runtime defaults should still be applied after a failed preferences load")
	}
}

// Image resolution has to happen on every render surface, because a relative
// asset path cannot resolve against the webview origin on any of them. Pinning
// all three from one fixture means adding a fourth unresolved surface fails a
// test that already exists (issue #18).
func TestImageAssetsResolveOnEveryRenderSurface(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: 'data:image/png;base64,SURFACE', exists: true }),
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; window.print = () => {}; 'ok'`, &res)
	evalJS(t, ctx, `(() => {
		const doc = window.__app.state.docs.find((item) => item.id === window.__app.state.activeDocId)
		doc.path = '/tmp/doc.md'
		return 'ok'
	})()`, &res)
	evalJS(t, ctx,
		"window.__app.setMarkdown('# Surfaces\\n\\n![photo](doc.assets/photo.png)\\n').then(() => 'ok')", &res)

	// 1. formatted editor
	var formatted bool
	evalJS(t, ctx, `(async () => {
		for (let i = 0; i < 100; i++) {
			if (document.querySelector('#wysiwyg img[src^="data:image/png;base64,SURFACE"]')) return true
			await new Promise((r) => setTimeout(r, 20))
		}
		return false
	})()`, &formatted)
	if !formatted {
		t.Error("formatted editor did not resolve the image asset")
	}

	// 2. split preview
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)
	var split bool
	evalJS(t, ctx, `(async () => {
		for (let i = 0; i < 100; i++) {
			if (document.querySelector('#split-preview img[src^="data:image/png;base64,SURFACE"]')) return true
			await new Promise((r) => setTimeout(r, 20))
		}
		return false
	})()`, &split)
	if !split {
		t.Error("split preview did not resolve the image asset")
	}

	// 3. print / export
	evalJS(t, ctx, "window.__app.printDocument('pdf').then(() => 'ok')", &res)
	var printed bool
	evalJS(t, ctx,
		`Boolean(document.querySelector('#print-root img[src^="data:image/png;base64,SURFACE"]'))`, &printed)
	if !printed {
		t.Error("print/export did not resolve the image asset")
	}
}

func TestSettingsModalAppliesBackedRuntimePreferences(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.__calls = [];
	globalThis.drmd = { native: {
		ListFontFamilies: async () => {
			globalThis.__calls.push(['fonts'])
			return ['Georgia', 'Fira Code', 'Menlo']
		},
		SetDirty: () => {},
		UpdateContent: () => {},
	} } ; 'ok'`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown('# Fonts\\n\\nBody text.\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.openSettings().then(() => 'ok')", &res)

	var modalOpen bool
	evalJS(t, ctx, `document.querySelector('[data-settings-modal]') !== null`, &modalOpen)
	if !modalOpen {
		t.Fatal("settings button should open the settings modal")
	}

	evalJS(t, ctx, "document.querySelector('[data-settings-nav=\"appearance\"]').click(); 'ok'", &res)
	var systemFontsLoaded bool
	evalJS(t, ctx, `globalThis.__calls.some((call) => call[0] === 'fonts') &&
		Array.from(document.querySelectorAll('[data-settings-field="codeFont"] option')).some((option) => option.value === 'Fira Code')`, &systemFontsLoaded)
	if !systemFontsLoaded {
		t.Fatal("code font options should come from the native installed-font bridge")
	}
	evalJS(t, ctx, `document.querySelector('[data-settings-theme="dark"]').click(); 'ok'`, &res)
	evalJS(t, ctx, `(() => {
		const documentFont = document.querySelector('[data-settings-field="documentFont"]')
		documentFont.value = 'Georgia'
		documentFont.dispatchEvent(new Event('change', { bubbles: true }))
		const codeFont = document.querySelector('[data-settings-field="codeFont"]')
		codeFont.value = 'Fira Code'
		codeFont.dispatchEvent(new Event('change', { bubbles: true }))
		const ligatures = document.querySelector('[data-settings-field="codeLigatures"]')
		ligatures.checked = false
		ligatures.dispatchEvent(new Event('change', { bubbles: true }))
		const size = document.querySelector('[data-settings-field="documentFontSize"]')
		size.value = '17'
		size.dispatchEvent(new Event('input', { bubbles: true }))
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "document.querySelector('[data-settings-action=\"save\"]').click(); 'ok'", &res)

	var applied bool
	evalJS(t, ctx, `document.body.classList.contains('dark') &&
		getComputedStyle(document.documentElement).getPropertyValue('--document-font').includes('Georgia') &&
		getComputedStyle(document.documentElement).getPropertyValue('--code-font').includes('Fira Code') &&
		getComputedStyle(document.documentElement).getPropertyValue('--document-font-size').trim() === '17px' &&
		getComputedStyle(document.documentElement).getPropertyValue('--code-ligatures').trim() === 'none' &&
		getComputedStyle(document.querySelector('#wysiwyg .ProseMirror')).fontFamily.includes('Georgia')`, &applied)
	if !applied {
		t.Fatal("saving settings should visibly apply theme, document font, code font, font size, and ligatures")
	}

	evalJS(t, ctx, "document.getElementById('btn-settings').click(); 'ok'", &res)
	evalJS(t, ctx, "document.querySelector('[data-settings-nav=\"editor\"]').click(); 'ok'", &res)
	evalJS(t, ctx, `(() => {
		const lineNumbers = document.querySelector('[data-settings-field="lineNumbers"]')
		lineNumbers.checked = false
		lineNumbers.dispatchEvent(new Event('change', { bubbles: true }))
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "document.querySelector('[data-settings-action=\"save\"]').click(); 'ok'", &res)
	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)

	var rawOptionApplied bool
	evalJS(t, ctx, `window.__app.state.rawOptions.lineNumbers === false &&
		getComputedStyle(document.querySelector('#raw .cm-gutters')).display === 'none'`, &rawOptionApplied)
	if !rawOptionApplied {
		t.Fatal("saved line-number preference should reconfigure raw mode")
	}
}

func TestSettingsCancelDiscardsDraftAndMarksFutureSectionsDisabled(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "document.getElementById('btn-settings').click(); 'ok'", &res)

	var futureSectionsDisabled bool
	evalJS(t, ctx, `['markdown','sync','extensions'].every((tab) => {
		const button = document.querySelector('[data-settings-nav="' + tab + '"]')
		return button && button.disabled && button.getAttribute('aria-disabled') === 'true'
	})`, &futureSectionsDisabled)
	if !futureSectionsDisabled {
		t.Fatal("future settings sections should be visible but disabled until backed")
	}

	evalJS(t, ctx, "document.querySelector('[data-settings-nav=\"appearance\"]').click(); 'ok'", &res)
	evalJS(t, ctx, `document.querySelector('[data-settings-theme="dark"]').click(); 'ok'`, &res)
	evalJS(t, ctx, "document.querySelector('[data-settings-action=\"cancel\"]').click(); 'ok'", &res)

	var cancelled bool
	evalJS(t, ctx, `!document.body.classList.contains('dark') &&
		document.querySelector('[data-settings-modal]') === null`, &cancelled)
	if !cancelled {
		t.Fatal("cancel should close settings without applying the draft")
	}

	evalJS(t, ctx, "window.__app.activateRibbonTab('view'); 'ok'", &res)
	var viewSettingsCount int
	evalJS(t, ctx, `document.querySelectorAll('[data-ribbon-panel="view"] #btn-settings').length`, &viewSettingsCount)
	if viewSettingsCount != 0 {
		t.Fatal("settings should not appear in the View ribbon context")
	}

	var tabRowSettings bool
	evalJS(t, ctx, `document.querySelector('.ribbon-tabs-row > #btn-settings') !== null`, &tabRowSettings)
	if !tabRowSettings {
		t.Fatal("settings should live in the ribbon tab row")
	}
}

func TestSettingsEditorDesignControlsAreBacked(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.openSettings().then(() => 'ok')", &res)
	var editorControls bool
	evalJS(t, ctx, `['defaultMode','showFormattedMarkers','editorWidth','formatOnSave','softWrap','lineNumbers'].every((field) =>
		document.querySelector('[data-settings-field="' + field + '"]'))`, &editorControls)
	if !editorControls {
		t.Fatal("Editor settings should expose backed design controls")
	}

	evalJS(t, ctx, `(() => {
		const mode = document.querySelector('[data-settings-field="defaultMode"]')
		mode.value = 'raw'
		mode.dispatchEvent(new Event('change', { bubbles: true }))
		const markers = document.querySelector('[data-settings-field="showFormattedMarkers"]')
		markers.checked = true
		markers.dispatchEvent(new Event('change', { bubbles: true }))
		const width = document.querySelector('[data-settings-field="editorWidth"]')
		width.value = '84'
		width.dispatchEvent(new Event('input', { bubbles: true }))
		const format = document.querySelector('[data-settings-field="formatOnSave"]')
		format.checked = true
		format.dispatchEvent(new Event('change', { bubbles: true }))
		document.querySelector('[data-settings-action="save"]').click()
		return 'ok'
	})()`, &res)

	var applied bool
	evalJS(t, ctx, `window.__app.state.settings.defaultMode === 'raw' &&
		window.__app.state.settings.showFormattedMarkers === true &&
		window.__app.state.settings.editorWidth === 84 &&
		window.__app.state.settings.formatOnSave === true &&
		document.body.classList.contains('show-formatted-markers') &&
		getComputedStyle(document.documentElement).getPropertyValue('--editor-width').trim() === '84ch'`, &applied)
	if !applied {
		t.Fatal("Editor settings design controls should save and apply backed runtime state")
	}

	evalJS(t, ctx, "window.__app.newDocument().then(() => 'ok')", &res)
	var mode string
	evalJS(t, ctx, "window.__app.state.mode", &mode)
	if mode != "raw" {
		t.Fatalf("default mode setting should apply to new documents, got %q", mode)
	}
}

func TestBootLoadsPersistedPreferencesThroughBridge(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootAppWithNativeStub(t, ctx, url, `globalThis.__nativeCalls = [];
	globalThis.drmd = { native: {
		LoadPreferences: async () => {
			globalThis.__nativeCalls.push(['loadPreferences'])
			return {
				settings: {
					theme: 'dark',
					defaultMode: 'split',
					documentFont: 'Georgia',
					documentFontSize: 18,
					codeFont: 'Menlo',
					codeLigatures: false,
					editorWidth: 88,
					showFormattedMarkers: true,
					formatOnSave: true
				},
				rawOptions: { softWrap: false, lineNumbers: false },
				recents: []
			}
		},
		SetDirty: async () => {},
		UpdateContent: async () => {},
		ListFontFamilies: async () => ['Georgia', 'Menlo']
	} } ;`)

	var applied bool
	evalJS(t, ctx, `globalThis.__nativeCalls.some((call) => call[0] === 'loadPreferences') &&
		window.__app.state.settings.theme === 'dark' &&
		window.__app.state.settings.defaultMode === 'split' &&
		window.__app.state.rawOptions.lineNumbers === false &&
		document.body.classList.contains('dark') &&
		document.body.classList.contains('show-formatted-markers') &&
		getComputedStyle(document.documentElement).getPropertyValue('--document-font').includes('Georgia') &&
		getComputedStyle(document.documentElement).getPropertyValue('--code-font').includes('Menlo') &&
		getComputedStyle(document.documentElement).getPropertyValue('--document-font-size').trim() === '18px' &&
		getComputedStyle(document.documentElement).getPropertyValue('--editor-width').trim() === '88ch'`, &applied)
	if !applied {
		t.Fatal("boot should load persisted settings and apply runtime preferences before first interaction")
	}
}

func TestSettingsSavePersistsNativePreferences(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootAppWithNativeStub(t, ctx, url, `globalThis.__savedPreferences = null;
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		SavePreferences: async (prefs) => { globalThis.__savedPreferences = prefs },
		ListFontFamilies: async () => ['Georgia', 'Menlo'],
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ;`)

	var res string
	evalJS(t, ctx, "window.__app.openSettings().then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => {
		const width = document.querySelector('[data-settings-field="editorWidth"]')
		width.value = '86'
		width.dispatchEvent(new Event('input', { bubbles: true }))
		const lineNumbers = document.querySelector('[data-settings-field="lineNumbers"]')
		lineNumbers.checked = false
		lineNumbers.dispatchEvent(new Event('change', { bubbles: true }))
		document.querySelector('[data-settings-action="save"]').click()
		return 'ok'
	})()`, &res)

	var saved bool
	evalJS(t, ctx, `globalThis.__savedPreferences &&
		globalThis.__savedPreferences.settings.editorWidth === 86 &&
		globalThis.__savedPreferences.rawOptions.lineNumbers === false &&
		Array.isArray(globalThis.__savedPreferences.recents)`, &saved)
	if !saved {
		t.Fatal("settings save should persist the full native preferences envelope")
	}
}

func TestRecentFilesRenderAndOpenSpecificRecentBridge(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootAppWithNativeStub(t, ctx, url, `globalThis.__recentOpened = '';
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({
			settings: {},
			rawOptions: {},
			recents: [{ path: '/tmp/recent.md', title: 'recent.md', lastOpenedAt: '2026-08-07T13:00:00Z' }]
		}),
		OpenRecentDocument: async (path) => {
			globalThis.__recentOpened = path
			return { path, content: '# Recent\n\nLoaded from native recents.\n' }
		},
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ;`)

	var recentRows []string
	evalJS(t, ctx, `Array.from(document.querySelectorAll('[data-recent-file]')).map((row) => row.textContent.trim())`, &recentRows)
	if len(recentRows) != 1 || !strings.Contains(recentRows[0], "recent.md") {
		t.Fatalf("recent files should render on the empty state: %v", recentRows)
	}

	var res string
	evalJS(t, ctx, `document.querySelector('[data-recent-file]').click(); 'ok'`, &res)
	var opened bool
	evalJS(t, ctx, `globalThis.__recentOpened === '/tmp/recent.md' &&
		window.__app.state.path === '/tmp/recent.md' &&
		window.__app.getMarkdown().includes('Loaded from native recents') &&
		!document.body.classList.contains('app-empty')`, &opened)
	if !opened {
		t.Fatal("clicking a recent file should call OpenRecentDocument and load that markdown")
	}
}

func TestEmptyStateActionsAndTemplatesAreBacked(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `Object.defineProperty(navigator, 'clipboard', {
		value: { readText: async () => '# Pasted\n\nClipboard body.\n' },
		configurable: true
	}); 'ok'`, &res)
	evalJS(t, ctx, "document.getElementById('empty-paste').click(); 'ok'", &res)

	var md string
	waitForJS(t, ctx, `window.__app.getMarkdown().includes('# Pasted')`)
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "# Pasted") {
		t.Fatalf("paste action did not load clipboard markdown: %q", md)
	}

	evalJS(t, ctx, "window.__app.newDocument().then(() => 'ok')", &res)
	evalJS(t, ctx, "document.querySelector('[data-template=\"readme\"]').click(); 'ok'", &res)
	waitForJS(t, ctx, `window.__app.getMarkdown().includes('# Project Name')`)
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "# Project Name") || !strings.Contains(md, "## Installation") {
		t.Fatalf("README template did not create markdown: %q", md)
	}
}

func TestFileRailSearchAndOutlinePanelsAreBacked(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.__openQueue = [
		{ path: '/tmp/alpha.md', content: '# Alpha\n\n## Beta\n\nSee [Docs](https://example.com).\n' },
		{ path: '/tmp/gamma.md', content: '# Gamma\n' }
	];
	globalThis.drmd = { native: {
		OpenDocument: async () => globalThis.__openQueue.shift(),
		SaveDocument: async () => {},
		SaveDocumentAs: async () => '/tmp/saved.md',
		SetDirty: () => {},
		UpdateContent: () => {},
	} } ; 'ok'`, &res)
	evalJS(t, ctx, "window.__app.openDocument().then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.newDocument().then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.openDocument().then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => {
		const input = document.getElementById('file-search-input')
		input.value = 'gamma'
		input.dispatchEvent(new Event('input', { bubbles: true }))
		return 'ok'
	})()`, &res)

	var visibleTitles []string
	evalJS(t, ctx, `Array.from(document.querySelectorAll('#file-list .rail-row'))
		.filter((row) => getComputedStyle(row).display !== 'none')
		.map((row) => row.textContent.trim())`, &visibleTitles)
	if len(visibleTitles) != 1 || !strings.Contains(visibleTitles[0], "gamma.md") {
		t.Fatalf("file rail search did not filter actual open buffers: %v", visibleTitles)
	}

	evalJS(t, ctx, "window.__app.activateDocument(window.__app.state.docs[0].id).then(() => 'ok')", &res)
	var outline []string
	evalJS(t, ctx, `Array.from(document.querySelectorAll('#outline-list .outline-row')).map((row) => row.textContent.trim())`, &outline)
	if len(outline) != 2 || outline[0] != "Alpha" || outline[1] != "Beta" {
		t.Fatalf("outline did not derive headings from markdown: %v", outline)
	}

	evalJS(t, ctx, "document.querySelector('[data-outline-tab=\"links\"]').click(); 'ok'", &res)
	var links []string
	evalJS(t, ctx, `Array.from(document.querySelectorAll('#outline-list .outline-row')).map((row) => row.textContent.trim())`, &links)
	if len(links) != 1 || !strings.Contains(links[0], "Docs") {
		t.Fatalf("links panel did not derive links from markdown: %v", links)
	}
}

func TestVisibleRibbonButtonsFitLabels(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Ribbon Fit\\n').then(() => 'ok')", &res)

	for _, tab := range []string{"home", "insert", "format", "view"} {
		evalJS(t, ctx, "window.__app.activateRibbonTab('"+tab+"'); 'ok'", &res)
		var allFit bool
		evalJS(t, ctx, `Array.from(document.querySelectorAll('.ribbon-panel:not([hidden]) button:not(:disabled)')).every((button) => {
			return button.clientWidth + 1 >= button.scrollWidth
		})`, &allFit)
		if !allFit {
			t.Fatalf("visible ribbon buttons overflow labels in %s tab", tab)
		}
	}
}

func TestRibbonIconSpacingAndBackedButtonsVisibility(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Ribbon Layout\\n').then(() => 'ok')", &res)

	var iconGapConsistent bool
	evalJS(t, ctx, `(() => {
		const gaps = Array.from(document.querySelectorAll('.ribbon-controls.labelled button .ribbon-icon'))
			.map((icon) => getComputedStyle(icon.parentElement).gap)
		return gaps.length > 0 && gaps.every((gap) => gap === gaps[0])
	})()`, &iconGapConsistent)
	if !iconGapConsistent {
		t.Fatal("ribbon icon/text gap should be consistent")
	}

	var backedButtonsVisible bool
	evalJS(t, ctx, `(() => {
		const shell = document.getElementById('app-shell').getBoundingClientRect()
		return Array.from(document.querySelectorAll('.ribbon-panel:not([hidden]) button:not([disabled])'))
			.every((button) => {
				const rect = button.getBoundingClientRect()
				return rect.right <= shell.right && rect.left >= shell.left
			})
	})()`, &backedButtonsVisible)
	if !backedButtonsVisible {
		t.Fatal("visible backed ribbon buttons should fit at the default concept width")
	}

	var orderedIconStacked bool
	evalJS(t, ctx, `(() => {
		const icon = document.querySelector('[data-command="numbered-list"]')
		const marker = getComputedStyle(icon, '::after')
		return marker.content.includes('\\a') && marker.whiteSpace === 'pre'
	})()`, &orderedIconStacked)
	if !orderedIconStacked {
		t.Fatal("numbered-list icon should stack 1/2/3 vertically")
	}

	var iconReport string
	evalJS(t, ctx, `(() => {
		const commands = ['link', 'table', 'code-block', 'mermaid']
		const missing = commands.filter((command) => {
			const icon = document.querySelector('[data-command="' + command + '"] .ribbon-icon use')
			const href = icon && (icon.getAttribute('href') || icon.getAttribute('xlink:href') || (icon.href && icon.href.baseVal))
			return !(href && href.includes('#icon-'))
		})
		const stale = document.querySelector('#ribbon .link-icon, #ribbon .table-icon, #ribbon .diagram-icon')
		return missing.length === 0 && !stale ? '' : 'missing=' + missing.join(',') + ' stale=' + Boolean(stale)
	})()`, &iconReport)
	if iconReport != "" {
		t.Fatalf("insert ribbon icons should use reusable SVG symbols instead of CSS drawings: %s", iconReport)
	}
}

func TestSidePanelsCanBeHiddenIndependently(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Panels\\n\\nBody\\n').then(() => 'ok')", &res)

	var hiddenState string
	evalJS(t, ctx, `(() => {
		document.querySelector('[data-panel-toggle="left"]').click()
		const leftHidden = document.body.classList.contains('left-panel-hidden')
		const rightVisible = getComputedStyle(document.getElementById('outline-panel')).display !== 'none'
		return leftHidden && rightVisible ? 'ok' : 'bad'
	})()`, &hiddenState)
	if hiddenState != "ok" {
		t.Fatalf("left panel should hide independently, got %q", hiddenState)
	}

	evalJS(t, ctx, `(() => {
		document.querySelector('[data-panel-toggle="right"]').click()
		const rightHidden = document.body.classList.contains('right-panel-hidden')
		const leftHidden = document.body.classList.contains('left-panel-hidden')
		return rightHidden && leftHidden ? 'ok' : 'bad'
	})()`, &hiddenState)
	if hiddenState != "ok" {
		t.Fatalf("right panel should hide without restoring left panel, got %q", hiddenState)
	}
	var labelledRestoreRails string
	evalJS(t, ctx, `(() => {
		const left = document.querySelector('#file-rail .panel-restore')
		const right = document.querySelector('#outline-panel .panel-restore')
		const leftWidth = document.getElementById('file-rail').getBoundingClientRect().width
		const rightWidth = document.getElementById('outline-panel').getBoundingClientRect().width
		const ok = left && right &&
			left.textContent.trim() === 'Files' &&
			right.textContent.trim() === 'Document' &&
			left.getAttribute('aria-label') === 'Show file panel' &&
			right.getAttribute('aria-label') === 'Show outline panel' &&
			leftWidth <= 46 && rightWidth <= 46
		return ok ? 'ok' : JSON.stringify({
			leftText: left && left.textContent.trim(),
			rightText: right && right.textContent.trim(),
			leftLabel: left && left.getAttribute('aria-label'),
			rightLabel: right && right.getAttribute('aria-label'),
			leftWidth,
			rightWidth,
		})
	})()`, &labelledRestoreRails)
	if labelledRestoreRails != "ok" {
		t.Fatalf("collapsed side panels should expose compact labelled restore rails: %s", labelledRestoreRails)
	}

	evalJS(t, ctx, "window.__app.setMode('raw').then(() => 'ok')", &res)
	var hiddenAfterModeSwitch bool
	evalJS(t, ctx, `document.body.classList.contains('left-panel-hidden') &&
		document.body.classList.contains('right-panel-hidden') &&
		document.querySelector('[data-panel-toggle="right"]') !== null`, &hiddenAfterModeSwitch)
	if !hiddenAfterModeSwitch {
		t.Fatal("panel visibility state and restore controls should survive mode switches")
	}

	evalJS(t, ctx, `(() => {
		document.querySelector('[data-panel-toggle="left"]').click()
		document.querySelector('[data-panel-toggle="right"]').click()
		return 'ok'
	})()`, &res)

	var restored bool
	evalJS(t, ctx, `!document.body.classList.contains('left-panel-hidden') &&
		!document.body.classList.contains('right-panel-hidden') &&
		getComputedStyle(document.getElementById('file-rail')).display !== 'none' &&
		getComputedStyle(document.getElementById('outline-panel')).display !== 'none'`, &restored)
	if !restored {
		t.Fatal("side panels should restore independently")
	}

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "# Panels") {
		t.Fatalf("panel visibility toggles should not mutate markdown: %q", md)
	}
}

func TestNativeInteractionAndAccessibilityState(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `window.__app.openSettings().then(() => document.activeElement?.dataset.settingsNav || '')`, &res)
	if res != "editor" {
		t.Fatalf("settings dialog should focus its first navigation control, got %q", res)
	}
	chromedp.Run(ctx, chromedp.KeyEvent("\u001b"))
	var closed bool
	evalJS(t, ctx, `document.querySelector('[data-settings-modal]') === null`, &closed)
	if !closed {
		t.Fatal("Escape should close settings")
	}

	evalJS(t, ctx, `document.getElementById('btn-file-menu').click(); 'ok'`, &res)
	chromedp.Run(ctx, chromedp.KeyEvent("\u001b"))
	evalJS(t, ctx, `document.querySelector('[data-export-menu]') === null`, &closed)
	if !closed {
		t.Fatal("Escape should close export menu")
	}

	evalJS(t, ctx, `document.querySelector('[data-help-toggle]').click(); 'ok'`, &res)
	chromedp.Run(ctx, chromedp.KeyEvent("\u001b"))
	evalJS(t, ctx, `document.querySelector('[data-help-panel]') === null`, &closed)
	if !closed {
		t.Fatal("Escape should close help panel")
	}

	evalJS(t, ctx, `document.querySelector('[data-command="code-block"]').click(); document.activeElement?.matches('[data-code-language]') ? 'ok' : ''`, &res)
	if res != "ok" {
		t.Fatalf("code assistant should focus the language selector, got %q", res)
	}
	chromedp.Run(ctx, chromedp.KeyEvent("\u001b"))
	evalJS(t, ctx, `document.querySelector('[data-code-assistant]') === null`, &closed)
	if !closed {
		t.Fatal("Escape should close code assistant")
	}

	evalJS(t, ctx, `document.querySelector('[data-command="mermaid"]').click(); document.activeElement?.dataset.diagramType || ''`, &res)
	if res == "" {
		t.Fatal("diagram assistant should focus a diagram type control")
	}
	chromedp.Run(ctx, chromedp.KeyEvent("\u001b"))
	evalJS(t, ctx, `document.querySelector('[data-diagram-assistant]') === null`, &closed)
	if !closed {
		t.Fatal("Escape should close diagram assistant")
	}

	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)
	// aria-CHECKED, not aria-pressed: the three modes became one radiogroup, so a
	// screen reader announces one choice of three rather than three toggles.
	var checked string
	evalJS(t, ctx, `[
		document.getElementById('btn-mode-formatted').getAttribute('aria-checked'),
		document.getElementById('btn-mode-raw').getAttribute('aria-checked'),
		document.getElementById('btn-split').getAttribute('aria-checked')
	].join(',')`, &checked)
	if checked != "false,false,true" {
		t.Fatalf("mode controls should expose aria-checked state, got %q", checked)
	}
}

func TestPrintAndPDFExportUseFormattedPrintPipeline(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := "# Printable\n\nParagraph with **strong** text.\n\n```javascript\nconst answer = 42\n```\n"
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)
	evalJS(t, ctx, `(() => { window.__printCalls = 0; window.print = () => { window.__printCalls += 1 }; return 'ok' })()`, &res)
	evalJS(t, ctx, "window.__app.setMode('split').then(() => 'ok')", &res)
	var beforePrint string
	evalJS(t, ctx, "window.__app.getMarkdown()", &beforePrint)

	evalJS(t, ctx, `document.getElementById('btn-file-menu').click(); 'ok'`, &res)
	evalJS(t, ctx, `document.querySelector('[data-file-menu-action="print"]').click(); 'ok'`, &res)
	var printState string
	evalJS(t, ctx, `[
		window.__printCalls,
		document.body.dataset.lastExportAction,
		document.querySelector('#print-root h1')?.textContent.trim(),
		document.querySelector('#print-root .code-block-shell[data-language="javascript"]') !== null,
		window.__app.state.mode,
		window.__app.state.dirty
	].join('|')`, &printState)
	if printState != "1|print|Printable|true|split|true" {
		t.Fatalf("print should use formatted print output without changing mode/dirty state, got %q", printState)
	}

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if md != beforePrint {
		t.Fatalf("print should not mutate markdown:\n%q", md)
	}

	evalJS(t, ctx, `document.getElementById('btn-file-menu').click(); 'ok'`, &res)
	evalJS(t, ctx, `document.querySelector('[data-file-menu-action="pdf"]').click(); 'ok'`, &res)
	var pdfState string
	evalJS(t, ctx, `[window.__printCalls, document.body.dataset.lastExportAction, document.body.dataset.pdfExportVia].join('|')`, &pdfState)
	if pdfState != "2|pdf|print-dialog" {
		t.Fatalf("PDF export should use the native print-to-PDF pipeline, got %q", pdfState)
	}
}

func TestVisualBaselineScreensRenderWithoutOverflow(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 820)); err != nil {
		t.Fatalf("set constrained viewport: %v", err)
	}

	states := []struct {
		name  string
		setup string
	}{
		{name: "empty", setup: `"ok"`},
		{name: "formatted", setup: `window.__app.setMarkdown("# Baseline\n\nParagraph\n").then(() => "ok")`},
		{name: "raw", setup: `window.__app.setMode("raw").then(() => "ok")`},
		{name: "split", setup: `window.__app.setMode("split").then(() => "ok")`},
		{name: "settings", setup: `window.__app.openSettings().then(() => "ok")`},
	}

	for _, state := range states {
		var res string
		evalJS(t, ctx, state.setup, &res)
		var layoutOK bool
		evalJS(t, ctx, `document.documentElement.scrollWidth <= window.innerWidth + 1`, &layoutOK)
		if !layoutOK {
			var overflowReport string
			evalJS(t, ctx, `JSON.stringify({
				innerWidth: window.innerWidth,
				scrollWidth: document.documentElement.scrollWidth,
				appWidth: document.getElementById('app-shell').getBoundingClientRect().width,
				ribbonWidth: document.getElementById('ribbon').getBoundingClientRect().width,
				exportRight: document.getElementById('btn-file-menu').getBoundingClientRect().right
			})`, &overflowReport)
			t.Fatalf("%s baseline overflows the viewport: %s", state.name, overflowReport)
		}
		var shot []byte
		if err := chromedp.Run(ctx, chromedp.FullScreenshot(&shot, 92)); err != nil {
			t.Fatalf("%s screenshot: %v", state.name, err)
		}
		if !imageHasVisualContent(t, shot) {
			t.Fatalf("%s screenshot appears blank", state.name)
		}
	}

	var exportVisible bool
	evalJS(t, ctx, `(() => {
		const button = document.getElementById('btn-file-menu')
		const rect = button.getBoundingClientRect()
		return rect.left >= 0 && rect.right <= window.innerWidth && rect.width > 0
	})()`, &exportVisible)
	if !exportVisible {
		t.Fatal("Export control should fit inside the constrained default window")
	}
}

func TestResponsiveShellAdaptsToReducedWindowWidths(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `window.__app.setMarkdown("# Responsive\n\nParagraph\n").then(() => "ok")`, &res)

	widths := []int64{1280, 980, 760}
	for _, width := range widths {
		if err := chromedp.Run(ctx, chromedp.EmulateViewport(width, 820)); err != nil {
			t.Fatalf("set viewport %d: %v", width, err)
		}
		var layoutOK bool
		evalJS(t, ctx, `document.documentElement.scrollWidth <= window.innerWidth + 1`, &layoutOK)
		if !layoutOK {
			var report string
			evalJS(t, ctx, `JSON.stringify({
				width: window.innerWidth,
				scrollWidth: document.documentElement.scrollWidth,
				ribbonWidth: document.getElementById('ribbon').getBoundingClientRect().width,
				export: document.getElementById('btn-file-menu').getBoundingClientRect().toJSON?.() || {}
			})`, &report)
			t.Fatalf("responsive shell should not overflow at %dpx: %s", width, report)
		}
		var exportVisible bool
		evalJS(t, ctx, `(() => {
			const rect = document.getElementById('btn-file-menu').getBoundingClientRect()
			return rect.left >= 0 && rect.right <= window.innerWidth && rect.width > 0
		})()`, &exportVisible)
		if !exportVisible {
			t.Fatalf("Export should remain fully visible at %dpx", width)
		}
	}

	if err := chromedp.Run(ctx, chromedp.EmulateViewport(760, 820)); err != nil {
		t.Fatalf("set narrow viewport: %v", err)
	}
	var collapsedAccessible bool
	evalJS(t, ctx, `Array.from(document.querySelectorAll('.ribbon-controls.labelled button')).every((button) => {
		const rect = button.getBoundingClientRect()
		return rect.width <= 44 && button.getAttribute('aria-label') && button.title
	})`, &collapsedAccessible)
	if !collapsedAccessible {
		t.Fatal("labelled ribbon buttons should collapse to icon-sized controls with aria labels and tooltips at narrow widths")
	}

	var canvasVisible bool
	evalJS(t, ctx, `document.getElementById('document-region').getBoundingClientRect().width >= 360`, &canvasVisible)
	if !canvasVisible {
		var report string
		evalJS(t, ctx, `JSON.stringify({
			documentRegion: document.getElementById('document-region').getBoundingClientRect().width,
			fileRail: document.getElementById('file-rail').getBoundingClientRect().width,
			outline: document.getElementById('outline-panel').getBoundingClientRect().width,
			empty: document.getElementById('empty-state').getBoundingClientRect().width
		})`, &report)
		t.Fatalf("reduced window should preserve a usable document canvas: %s", report)
	}
}

func imageHasVisualContent(t *testing.T, data []byte) bool {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	bounds := img.Bounds()
	var differentPixels int
	first := img.At(bounds.Min.X, bounds.Min.Y)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 16 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 16 {
			if img.At(x, y) != first {
				differentPixels++
			}
			if differentPixels > 20 {
				return true
			}
		}
	}
	return false
}

func TestTabsPreserveIndependentBuffers(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# First\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.newDocument().then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.setMarkdown('# Second\\n').then(() => 'ok')", &res)

	var count int
	evalJS(t, ctx, "window.__app.state.docs.length", &count)
	if count != 2 {
		t.Fatalf("tab count = %d, want 2", count)
	}

	evalJS(t, ctx, "window.__app.activateDocument(window.__app.state.docs[0].id).then(() => 'ok')", &res)
	var first string
	evalJS(t, ctx, "window.__app.getMarkdown()", &first)
	if !strings.Contains(first, "# First") {
		t.Errorf("first tab markdown = %q", first)
	}

	evalJS(t, ctx, "window.__app.activateDocument(window.__app.state.docs[1].id).then(() => 'ok')", &res)
	var second string
	evalJS(t, ctx, "window.__app.getMarkdown()", &second)
	if !strings.Contains(second, "# Second") {
		t.Errorf("second tab markdown = %q", second)
	}
}

func TestRibbonShortcutUsesCommandPath(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, "window.__app.setMarkdown('# Shortcuts\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, `document.dispatchEvent(new KeyboardEvent('keydown', {
		key: 'b',
		metaKey: true,
		bubbles: true,
		cancelable: true
	})); 'ok'`, &res)

	var md string
	evalJS(t, ctx, "window.__app.getMarkdown()", &md)
	if !strings.Contains(md, "**bold text**") {
		t.Errorf("shortcut did not run bold command, markdown = %q", md)
	}
}

func TestRoundTripCorpus(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	files, err := filepath.Glob(filepath.Join("..", "testdata", "roundtrip", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no round-trip fixtures found: %v", err)
	}

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			a := string(data)
			aJSON, _ := json.Marshal(a)

			var b string
			evalJS(t, ctx,
				"window.__app.setMarkdown("+string(aJSON)+").then(() => window.__app.getEditorMarkdown())",
				&b)

			bJSON, _ := json.Marshal(b)
			var c string
			evalJS(t, ctx,
				"window.__app.setMarkdown("+string(bJSON)+").then(() => window.__app.getEditorMarkdown())",
				&c)

			if b != c {
				t.Errorf("unstable round-trip:\n--- B ---\n%q\n--- C ---\n%q", b, c)
			}
			if strings.HasSuffix(f, ".canonical.md") && a != b {
				t.Errorf("canonical fixture rewritten:\n--- A ---\n%q\n--- B ---\n%q", a, b)
			}
		})
	}
}

// Go must be told which document every write targets. The close guard used to
// pair a single dirty boolean with whatever path Go had last opened, which
// wrote a new tab's text over the previously opened file.
func TestFrontendReportsEveryTabWithItsOwnPath(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var res string
	evalJS(t, ctx, `globalThis.__synced = null;
	globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		SyncDocuments: async (docs) => { globalThis.__synced = docs },
		SetDirty: async () => {},
		UpdateContent: async () => {}
	} } ; 'ok'`, &res)

	// Tab one, given a path and edited.
	evalJS(t, ctx, `(() => {
		const doc = window.__app.state.docs.find((d) => d.id === window.__app.state.activeDocId)
		doc.path = '/tmp/notes.md'
		return 'ok'
	})()`, &res)
	evalJS(t, ctx, "window.__app.setMarkdown('# Notes\\n').then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.debugSimulateEdit('# Notes edited\\n'); 'ok'", &res)

	// Tab two: a new, pathless document with different content.
	evalJS(t, ctx, "window.__app.newDocument().then(() => 'ok')", &res)
	evalJS(t, ctx, "window.__app.debugSimulateEdit('scratch from another tab\\n'); 'ok'", &res)

	var synced string
	evalJS(t, ctx, "JSON.stringify(globalThis.__synced || [])", &synced)

	if !strings.Contains(synced, "/tmp/notes.md") {
		t.Fatalf("the first tab's path was not reported to Go: %s", synced)
	}
	if !strings.Contains(synced, "scratch from another tab") {
		t.Fatalf("the second tab's content was not reported: %s", synced)
	}
	// The decisive property: the pathless tab's content must not be attached
	// to the other tab's path anywhere in the payload.
	if strings.Contains(synced, `"path":"/tmp/notes.md","content":"scratch from another tab`) {
		t.Fatalf("a tab's content was reported against another tab's path: %s", synced)
	}
}

// clickWhenVisible waits for a selector to be present AND visible, then clicks
// it under a deadline.
//
// chromedp.Click has NO timeout. It waits for a matching, visible node forever,
// so an element that never becomes visible does not fail a test — it hangs the
// whole package until the binary's 10-minute panic, and every other test's
// result dies with it. That is exactly how
// TestMermaidRendersAndStaysEditableInFormattedMode took CI down: 7 minutes
// blocked in one Click, then `FAIL dr-markdown/e2e 600.011s` with no indication
// of which assertion was even involved.
//
// The visibility check is separate from the click on purpose. chromedp's own
// wait is silent about WHY it is waiting, and several affordances in this editor
// are revealed on hover or mounted lazily on intersection — so "present but
// zero-height" is a state worth naming in the failure rather than sitting in.
func clickWhenVisible(t *testing.T, ctx context.Context, selector string) {
	t.Helper()
	visible := `(() => {
		const e = document.querySelector(` + strconv.Quote(selector) + `)
		if (!e) return false
		const r = e.getBoundingClientRect()
		return r.height > 0 && r.width > 0
	})()`
	// A longer budget than waitForJS's two seconds, deliberately. Becoming
	// visible can mean a lazy mount driven by an IntersectionObserver, a Vue
	// re-render after a toggle, or a font load settling a zero-height box — none
	// of which are instant on a loaded CI runner, and all of which are correct
	// behaviour rather than a defect. Two seconds failed
	// TestMermaidRendersAndStaysEditableInFormattedMode on macOS while passing
	// locally, which is a statement about the runner, not the editor. The click
	// below already allows twenty; the wait for the thing to be clickable should
	// not be an order of magnitude stingier.
	if !waitForVisible(t, ctx, visible) {
		var present bool
		evalJS(t, ctx, `document.querySelector(`+strconv.Quote(selector)+`) !== null`, &present)
		if present {
			t.Fatalf("%s is in the DOM but never became visible, so it cannot be clicked "+
				"(hover-revealed, or lazily mounted and never intersected)", selector)
		}
		t.Fatalf("%s never appeared, so it cannot be clicked", selector)
	}
	tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := chromedp.Run(tctx, chromedp.Click(selector, chromedp.ByQuery)); err != nil {
		t.Fatalf("clicking %s: %v", selector, err)
	}
}

// waitForVisible polls a boolean expression on a budget suited to layout
// settling, rather than waitForJS's two seconds.
func waitForVisible(t *testing.T, ctx context.Context, expr string) bool {
	t.Helper()
	var ok bool
	evalJS(t, ctx, `(async () => {
		for (let i = 0; i < 300; i++) {
			if (`+expr+`) return true
			await new Promise((resolve) => setTimeout(resolve, 50))
		}
		return false
	})()`, &ok)
	return ok
}

// sendKeysTo types into a selector under the same deadline, for the same reason.
func sendKeysTo(t *testing.T, ctx context.Context, selector, keys string) {
	t.Helper()
	tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := chromedp.Run(tctx, chromedp.SendKeys(selector, keys, chromedp.ByQuery)); err != nil {
		t.Fatalf("typing into %s: %v", selector, err)
	}
}
