package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The project's own README, driven through the application.
//
// It is here because that is how the image-distortion defect (#131) was actually
// found: not by a fixture, but by opening a real document and switching to
// Split. Every synthetic fixture in this suite is something someone thought to
// write down, so it can only catch what was already imagined. The README is a
// document nobody shaped for a test — badges, an image, tables, fenced code in
// several languages, nested lists, links — and it changes as the product does,
// so it keeps finding things after the fixtures have gone quiet.
//
// What it deliberately does NOT do is assert byte-exact round-trip fidelity.
// 23-realistic-document.md exists for that and is FROZEN, which is what lets it
// pin exact known losses. Asserting the same against a living README would break
// on ordinary edits to the README and teach everyone to ignore it. This asserts
// structural properties instead: the constructs survive, and nothing collapses
// or distorts across a mode change.
func readmeSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the project README: %v", err)
	}
	if len(data) < 500 {
		t.Fatalf("README is %d bytes; too small to be the real document", len(data))
	}
	return string(data)
}

// Every image in the README keeps its geometry across a mode round trip, at a
// zoom that is not 1.
//
// The zoom is the variable that mattered: the defect was invisible at 100%
// because the double-counted scale factor was 1. It only appeared after a mode
// change, because that is when the editor rebuilds and the image loads again
// with a zoom already in effect.
func TestReadmeImagesSurviveAModeRoundTrip(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	src, _ := json.Marshal(readmeSource(t))
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+string(src)+").then(() => 'ok')", &res)

	type shot struct {
		Count int       `json:"count"`
		Width []float64 `json:"width"`
		Ratio []float64 `json:"ratio"`
	}
	var m struct{ Before, After shot }
	evalJS(t, ctx, `(async () => {
		// The images are remote badge URLs and a local banner; only the ones
		// that actually lay out are measured, because a badge that never loads
		// has no geometry to preserve and is not what this is about.
		const probe = () => {
			const imgs = Array.from(document.querySelectorAll('#wysiwyg img'))
				.filter((i) => i.getBoundingClientRect().width > 0)
			return {
				count: imgs.length,
				width: imgs.map((i) => Math.round(i.getBoundingClientRect().width)),
				ratio: imgs.map((i) => {
					const r = i.getBoundingClientRect()
					return r.height > 0 ? Math.round((r.width / r.height) * 100) / 100 : 0
				}),
			}
		}
		await new Promise((r) => setTimeout(r, 1600))
		document.documentElement.style.setProperty('--doc-zoom', '0.9')
		await new Promise((r) => setTimeout(r, 400))
		const Before = probe()
		await window.__app.setMode('split')
		await new Promise((r) => setTimeout(r, 600))
		await window.__app.setMode('wysiwyg')
		await new Promise((r) => setTimeout(r, 1200))
		const After = probe()
		document.documentElement.style.removeProperty('--doc-zoom')
		return { Before, After }
	})()`, &m)

	if m.Before.Count == 0 {
		t.Skip("no README image laid out in this environment; nothing to measure")
	}
	if m.After.Count != m.Before.Count {
		t.Fatalf("images disappeared across a mode round trip: %d -> %d", m.Before.Count, m.After.Count)
	}
	for i := range m.Before.Width {
		// A tolerance, not equality: sub-pixel layout differences are not what
		// this is looking for. The defect it guards against shrank an image by
		// ten percent per round trip.
		before, after := m.Before.Width[i], m.After.Width[i]
		if before == 0 {
			continue
		}
		drift := (after - before) / before
		if drift < -0.02 || drift > 0.02 {
			t.Errorf("image %d changed width across a mode round trip: %.0f -> %.0f (%.1f%%)",
				i, before, after, drift*100)
		}
		if m.Before.Ratio[i] > 0 && m.After.Ratio[i] > 0 {
			r := m.After.Ratio[i] / m.Before.Ratio[i]
			if r < 0.98 || r > 1.02 {
				t.Errorf("image %d was distorted across a mode round trip: aspect %.2f -> %.2f",
					i, m.Before.Ratio[i], m.After.Ratio[i])
			}
		}
	}
}

// The README's constructs all reach the formatted surface. A document this size
// exercises the paths a one-construct fixture cannot: constructs adjacent to
// each other, inside each other, and repeated.
func TestReadmeRendersItsConstructsInTheEditor(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	source := readmeSource(t)
	src, _ := json.Marshal(source)
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+string(src)+").then(() => 'ok')", &res)

	var counts map[string]int
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 1600))
		const w = document.getElementById('wysiwyg')
		return {
			h1: w.querySelectorAll('h1').length,
			h2: w.querySelectorAll('h2').length,
			code: w.querySelectorAll('.milkdown-code-block, pre').length,
			list: w.querySelectorAll('ul li, ol li').length,
			link: w.querySelectorAll('a').length,
			table: w.querySelectorAll('table').length,
		}
	})()`, &counts)

	// Derived from the source, not hard-coded: the README changes, and a test
	// that pins today's counts would be edited to match rather than believed.
	want := map[string]int{
		"h1":    strings.Count(source, "\n# ") + boolToInt(strings.HasPrefix(source, "# ")),
		"h2":    strings.Count(source, "\n## "),
		"table": strings.Count(source, "\n| --- |") + strings.Count(source, "\n|---|"),
	}
	for _, key := range []string{"h1", "h2"} {
		if want[key] > 0 && counts[key] == 0 {
			t.Errorf("README has %d %s in source but the editor rendered none", want[key], key)
		}
	}
	for _, key := range []string{"code", "list", "link"} {
		if counts[key] == 0 {
			t.Errorf("the editor rendered no %s from the README", key)
		}
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
