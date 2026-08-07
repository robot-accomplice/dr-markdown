// Command screenshots captures current README screenshots from frontend/dist.
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const sampleMarkdown = `# Dr. Markdown

Build a native markdown document with formatted editing, raw source, code, diagrams, and tables.

## Code

` + "```go" + `
func save(path string, content string) error {
	return document.WriteAtomic(path, content)
}
` + "```" + `

## Mermaid Diagram

` + "```mermaid" + `
graph TD
  A[Draft] --> B{Review}
  B -->|Ready| C[Publish]
  B -->|Needs work| A
` + "```" + `

| Area | Status |
| --- | --- |
| Preferences | Persisted |
| Recents | Native |
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root := filepath.Join("frontend", "dist")
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		return fmt.Errorf("frontend assets missing: %w", err)
	}
	outDir := filepath.Join("docs", "assets", "screenshots")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create screenshot dir: %w", err)
	}

	srv := httptest.NewServer(http.FileServer(http.Dir(root)))
	defer srv.Close()

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), chromedp.DefaultExecAllocatorOptions[:]...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(nativeStub()).Do(ctx)
			return err
		}),
		chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(srv.URL),
		chromedp.Poll("window.__app && window.__app.ready === true", nil),
	); err != nil {
		return fmt.Errorf("boot app: %w", err)
	}

	states := []struct {
		name  string
		setup string
	}{
		{name: "start", setup: `"ok"`},
		{name: "formatted", setup: `window.__app.setMarkdown(` + quoteJS(sampleMarkdown) + `).then(() => "ok")`},
		{name: "raw", setup: `window.__app.setMode("raw").then(() => "ok")`},
		{name: "split", setup: `window.__app.setMode("split").then(() => "ok")`},
		{name: "settings", setup: `window.__app.openSettings().then(() => "ok")`},
	}

	for _, state := range states {
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(state.setup, nil, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
				return p.WithAwaitPromise(true)
			}),
			chromedp.Sleep(250*time.Millisecond),
		); err != nil {
			return fmt.Errorf("prepare %s: %w", state.name, err)
		}
		var shot []byte
		if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&shot)); err != nil {
			return fmt.Errorf("capture %s: %w", state.name, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, state.name+".png"), shot, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", state.name, err)
		}
	}
	return nil
}

func quoteJS(s string) string {
	return strconv.Quote(s)
}

func nativeStub() string {
	return `globalThis.go = { main: { App: {
		LoadPreferences: async () => ({
			settings: {},
			rawOptions: {},
			recents: [
				{ path: '/Users/robot/Documents/architecture-notes.md', title: 'architecture-notes.md', lastOpenedAt: '2026-08-07T13:00:00Z' },
				{ path: '/Users/robot/Documents/release-plan.md', title: 'release-plan.md', lastOpenedAt: '2026-08-07T12:00:00Z' }
			]
		}),
		SavePreferences: async () => {},
		OpenRecentDocument: async (path) => ({ path, content: '# Recent\n\nOpened from README screenshot stub.\n' }),
		SetDirty: async () => {},
		UpdateContent: async () => {},
		ListFontFamilies: async () => ['Georgia', 'Menlo', 'Fira Code', 'Public Sans']
	} } };`
}
