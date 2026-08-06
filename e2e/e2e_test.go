// Package e2e drives the vendored frontend in headless Chrome via chromedp.
// It serves frontend/dist over httptest (matching the Wails asset-server
// environment) and talks to the page through the window.__app hooks.
//
// Requires a Chrome/Chromium binary on the machine; tests skip if none is
// found. Pure Go — no Node involved.
package e2e

import (
	"context"
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
