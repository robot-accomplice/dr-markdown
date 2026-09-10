package e2e

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

// Content must not MORPH as a function of zoom or of mode changes.
//
// #131 was exactly this and was found by hand: an image shrank by ten percent
// on every switch to Split, because the node view measured its container with
// getBoundingClientRect — which reports ZOOMED pixels — and wrote an explicit
// height back inside that same scaled context, counting the zoom twice. It was
// invisible at 100% because the factor was 1, and invisible in the fixtures
// because no fixture combined a real document with a zoom and a mode change.
//
// The invariant this pins is arithmetic rather than aesthetic. Document zoom is
// CSS `zoom` on the editor host, so at zoom Z an element's rect is its layout
// size times Z. Divide the rect by Z and the number must not move — not when the
// zoom changes, and not when the mode round-trips at a fixed zoom. Anything that
// drifts is being measured in one coordinate space and written in another, which
// is the defect class rather than one instance of it.
//
// Driven with the project's own README because that is how #131 was found. Every
// fixture is something someone thought to write down, so it can only catch what
// was already imagined.
func TestReadmeContentDoesNotMorphAcrossZoomAndMode(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	// The README's images are relative paths, and a relative path only becomes
	// a real image through the bridge. Without this stub every one of them
	// renders as a MISSING-ASSET PLACEHOLDER — which app.css gives a fixed
	// min-width, making it immune to the container-measuring logic that #131
	// lived in. Measured: the sweep passed with that defect deliberately
	// reinstated, because it was measuring fourteen placeholders.
	var stub string
	evalJS(t, ctx, `globalThis.drmd = { native: {
		LoadPreferences: async () => ({ settings: {}, rawOptions: {}, recents: [] }),
		LoadImageAsset: async () => ({ dataURI: 'data:image/png;base64,`+wideImageB64+`', exists: true }),
		SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
	} } ; 'ok'`, &stub)

	src, _ := json.Marshal(readmeSource(t))
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+string(src)+").then(() => 'ok')", &res)

	// The probe reports every measurable content element, already divided by the
	// zoom in force. Elements are keyed by kind and ordinal rather than by
	// identity, because a mode round trip rebuilds the editor and the nodes are
	// not the same objects afterwards.
	const probe = `(() => {
		const z = parseFloat(getComputedStyle(document.documentElement)
			.getPropertyValue('--doc-zoom')) || 1
		const out = {}
		const add = (kind, els, fn) => els.forEach((el, i) => {
			const v = fn(el)
			if (v !== null) out[kind + ':' + i] = v
		})
		const host = document.getElementById('wysiwyg')
		const w = (el) => {
			const r = el.getBoundingClientRect()
			return r.width > 0 ? Math.round((r.width / z) * 10) / 10 : null
		}
		add('img', Array.from(host.querySelectorAll('img')), w)
		add('table', Array.from(host.querySelectorAll('table')), w)
		add('code', Array.from(host.querySelectorAll('.milkdown-code-block, pre')), w)
		add('svg', Array.from(host.querySelectorAll('.mermaid-render svg')), w)
		// Computed font-size is NOT divided by the zoom, and rects are. CSS
		// CSS zoom scales the rendered output; it does not rewrite the computed
		// style, so getComputedStyle keeps reporting the authored px at every
		// zoom level while getBoundingClientRect reports the scaled result.
		// Normalising both the same way reported a 25% drift that was purely an
		// artefact of the measurement.
		add('h1size', Array.from(host.querySelectorAll('h1')), (el) =>
			Math.round(parseFloat(getComputedStyle(el).fontSize) * 10) / 10)
		add('h2size', Array.from(host.querySelectorAll('h2')), (el) =>
			Math.round(parseFloat(getComputedStyle(el).fontSize) * 10) / 10)
		// Heading BOXES are rects, so they are normalised like every other rect.
		// Between them these catch a heading that changes size and one whose box
		// changes without its font following.
		add('h1box', Array.from(host.querySelectorAll('h1')), w)
		add('h2box', Array.from(host.querySelectorAll('h2')), w)
		// Aspect ratio is tracked separately: a distorted image can keep its
		// width while its height is wrong, which is what "morphing" looks like.
		Array.from(host.querySelectorAll('img')).forEach((el, i) => {
			const r = el.getBoundingClientRect()
			if (r.width > 0 && r.height > 0) {
				out['aspect:' + i] = Math.round((r.width / r.height) * 100) / 100
			}
		})
		return out
	})()`

	settle := func() {
		var ok string
		evalJS(t, ctx, `(async () => { await new Promise((r) => setTimeout(r, 1200)); return 'ok' })()`, &ok)
	}
	setZoom := func(z float64) {
		var ok string
		evalJS(t, ctx, fmt.Sprintf(
			`(async () => { document.documentElement.style.setProperty('--doc-zoom','%g');
			 await new Promise((r) => setTimeout(r, 400)); return 'ok' })()`, z), &ok)
	}
	measure := func() map[string]float64 {
		var m map[string]float64
		evalJS(t, ctx, probe, &m)
		return m
	}

	settle()

	// The reference: 100%, formatted, untouched.
	setZoom(1)
	settle()
	baseline := measure()
	if len(baseline) == 0 {
		t.Fatal("nothing measurable rendered from the README")
	}
	// A sweep with no real images cannot catch an image-sizing defect, and that
	// is not a hypothetical: this test passed with #131 reinstated until the
	// bridge stub above made the images real. Refuse to report a pass on a
	// document whose images never loaded.
	var loaded int
	evalJS(t, ctx, `Array.from(document.querySelectorAll('#wysiwyg img'))
		.filter((i) => (i.getAttribute('src') || '').startsWith('data:image')).length`, &loaded)
	if loaded == 0 {
		t.Fatal("no README image resolved to a real bitmap; this sweep cannot see an image defect")
	}
	t.Logf("%d README images resolved to real bitmaps", loaded)
	t.Logf("baseline at zoom 1.0, formatted: %d measured elements", len(baseline))

	// A drift of more than 2% is a defect. #131 moved things by 10% per switch,
	// and sub-pixel layout noise is well under 1%.
	const tolerance = 0.02
	compare := func(label string, got map[string]float64) {
		for key, want := range baseline {
			have, ok := got[key]
			if !ok {
				t.Errorf("%s: %s disappeared", label, key)
				continue
			}
			if want == 0 {
				continue
			}
			drift := math.Abs(have-want) / want
			if drift > tolerance {
				t.Errorf("%s: %s morphed %.1f%% — %.1f at baseline, %.1f here",
					label, key, drift*100, want, have)
			}
		}
		for key := range got {
			if _, ok := baseline[key]; !ok {
				t.Errorf("%s: %s appeared, which the baseline did not have", label, key)
			}
		}
	}

	// 1. Zoom alone. Normalized geometry must be identical at every level.
	for _, z := range []float64{0.8, 0.9, 1.1, 1.25, 1.5, 1} {
		setZoom(z)
		settle()
		compare(fmt.Sprintf("zoom %.2f (formatted)", z), measure())
	}

	// 2. Mode round trips at 100%. Returning to formatted must return the
	//    geometry with it.
	for _, trip := range [][]string{
		{"split", "wysiwyg"},
		{"raw", "wysiwyg"},
		{"split", "raw", "wysiwyg"},
		{"raw", "split", "wysiwyg"},
	} {
		for _, mode := range trip {
			var ok string
			evalJS(t, ctx, "window.__app.setMode("+jsonString(mode)+").then(() => 'ok')", &ok)
		}
		settle()
		compare(fmt.Sprintf("after round trip %v at zoom 1.00", trip), measure())
	}

	// 3. The combination that found #131: a zoom that is not 1, THEN a mode
	//    round trip. This is the one that failed, and it failed only here.
	for _, z := range []float64{0.9, 1.25} {
		setZoom(z)
		settle()
		for _, mode := range []string{"split", "wysiwyg"} {
			var ok string
			evalJS(t, ctx, "window.__app.setMode("+jsonString(mode)+").then(() => 'ok')", &ok)
		}
		settle()
		compare(fmt.Sprintf("zoom %.2f then split round trip", z), measure())

		// And again — #131 compounded, so one trip was not enough to see it.
		for i := 0; i < 3; i++ {
			for _, mode := range []string{"split", "wysiwyg"} {
				var ok string
				evalJS(t, ctx, "window.__app.setMode("+jsonString(mode)+").then(() => 'ok')", &ok)
			}
		}
		settle()
		compare(fmt.Sprintf("zoom %.2f after four split round trips", z), measure())
	}

	var reset string
	evalJS(t, ctx, `(() => { document.documentElement.style.removeProperty('--doc-zoom'); return 'ok' })()`, &reset)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
