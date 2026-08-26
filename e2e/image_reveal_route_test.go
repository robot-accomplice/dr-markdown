package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// Reveal in Finder has a route, and says why when it does not apply.
//
// #85. The floating contextual bar was removed in #81, and its image group went
// with it. Every other control in that group duplicated something the editor
// already provides — the resize handle, the caption, the upload affordance,
// selecting and deleting the block — but Reveal did not, because it calls
// through to the host to show the asset and no editor plugin can do that. The
// capability survived; only the affordance was lost.
//
// It is on the View menu rather than on the image block because that block is
// entirely vendored: appending to its controls means injecting into DOM the
// node view owns and rebuilds, which is the pattern behind #77 and #80.
//
// A menu cannot know whether an image is selected. So the case that matters
// most is the one where nothing is: a command that silently does nothing is
// indistinguishable from a broken one, which is exactly #75.
func TestRevealInFinderSaysWhyWhenNoImageIsSelected(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{"# No images here", "", "Just prose.", ""}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	var message string
	evalJS(t, ctx, `(async () => {
		await window.__app.revealSelectedImage()
		await new Promise((r) => setTimeout(r, 200))
		const el = document.getElementById('status-message')
		if (!el) return '(no status area)'
		return el.classList.contains('showing') ? el.textContent : '(nothing shown)'
	})()`, &message)

	if message == "(no status area)" {
		t.Fatal("there is nowhere for the application to say anything, so a menu command that " +
			"does not apply can only fail silently")
	}
	if message == "(nothing shown)" {
		t.Error("Reveal in Finder did nothing and said nothing with no image selected: a command " +
			"that silently no-ops cannot be told apart from one that is broken")
	}
	t.Logf("with no image selected, the app says: %q", message)
}

// The command still reaches the host when an image IS selected. The capability
// was never lost — only its affordance — and this is what proves the route
// reconnects to it rather than merely existing.
func TestRevealInFinderReachesTheHostForASelectedImage(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{"![a figure](figure.png)", ""}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	var outcome string
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 500))
		const img = document.querySelector('#wysiwyg img')
		if (!img) return '(no image rendered)'
		// Selecting the image is what the app's own click handler does; the menu
		// acts on whatever is selected.
		img.click()
		await new Promise((r) => setTimeout(r, 150))

		// Record what the bridge is asked for. There is no host in a browser, so
		// globalThis.drmd.native does not exist and the binding resolves to null
		// rather than throwing — stand one up, or this measures the absence of a
		// host instead of the behaviour of the command.
		const calls = []
		globalThis.drmd = globalThis.drmd || {}
		const previous = globalThis.drmd.native
		globalThis.drmd.native = Object.assign({}, previous, {
			RevealImageAsset: (doc, path) => { calls.push(path); return Promise.resolve() },
		})
		await window.__app.revealSelectedImage()
		globalThis.drmd.native = previous
		await new Promise((r) => setTimeout(r, 150))
		if (calls.length) return 'asked for ' + calls[0]
		const el = document.getElementById('status-message')
		return 'no call; status says ' + JSON.stringify(el ? el.textContent : null)
	})()`, &outcome)

	t.Logf("%s", outcome)
	if !strings.HasPrefix(outcome, "asked for ") {
		t.Errorf("with an image selected, Reveal in Finder did not reach the host: %s", outcome)
	}
	if !strings.Contains(outcome, "figure.png") {
		t.Errorf("the host was asked for the wrong asset: %s", outcome)
	}
}
