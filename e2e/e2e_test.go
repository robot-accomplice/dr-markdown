// Package e2e drives the vendored frontend in headless Chrome via chromedp.
// It serves frontend/dist over httptest (matching the Wails asset-server
// environment) and talks to the page through the window.__app hooks.
//
// Requires a Chrome/Chromium binary on the machine; tests skip if none is
// found. Pure Go — no Node involved.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func chromeAvailable() bool {
	if _, err := os.Stat("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"); err == nil {
		return true // macOS default install
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		for _, name := range []string{"google-chrome", "chrome", "chromium", "chromium-browser", "chrome-headless-shell"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return true
			}
		}
	}
	return false
}

func newTestBrowser(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	if !chromeAvailable() {
		t.Skip("no Chrome/Chromium binary found; skipping e2e")
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		chromedp.DefaultExecAllocatorOptions[:]...)
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
	if !strings.Contains(md, "Welcome") {
		t.Errorf("getMarkdown() = %q, want it to contain the welcome document", md)
	}

	var mode string
	evalJS(t, ctx, "window.__app.state.mode", &mode)
	if mode != "wysiwyg" {
		t.Errorf("mode = %q, want wysiwyg", mode)
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

func TestFileFlowWithStubBridge(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	stub := `globalThis.__calls = [];
globalThis.go = { main: { App: {
  OpenDocument: async () => ({ path: '/tmp/fake.md', content: "# From Stub\n" }),
  SaveDocument: async (p, c) => { globalThis.__calls.push(['save', p, c]); },
  SaveDocumentAs: async (c) => { globalThis.__calls.push(['saveAs', c]); return '/tmp/fake.md'; },
  SetDirty: (d) => { globalThis.__calls.push(['dirty', d]); },
  UpdateContent: (c) => { globalThis.__calls.push(['content', c]); },
} } }; 'ok'`
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
globalThis.go = { main: { App: {
  SetDirty: (d) => { globalThis.__calls.push(['dirty', d]); },
  UpdateContent: (c) => { globalThis.__calls.push(['content', c]); },
} } }; 'ok'`
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
globalThis.go = { main: { App: {
  SetDirty: (d) => { globalThis.__calls.push(['dirty', d]); },
  UpdateContent: (c) => { globalThis.__calls.push(['content', c]); },
} } }; 'ok'`
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
globalThis.go = { main: { App: {
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
} } }; 'ok'`
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
				"window.__app.setMarkdown("+string(aJSON)+").then(() => window.__app.getMarkdown())",
				&b)

			bJSON, _ := json.Marshal(b)
			var c string
			evalJS(t, ctx,
				"window.__app.setMarkdown("+string(bJSON)+").then(() => window.__app.getMarkdown())",
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
