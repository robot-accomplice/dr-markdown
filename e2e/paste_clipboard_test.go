package e2e

import "testing"

// A denied clipboard must not kill the Paste button.
//
// pasteMarkdown reads the system clipboard, and macOS refuses a read with no
// user gesture behind it — the promise REJECTS rather than resolving empty.
// Optional chaining guards absence, not refusal, so the rejection escaped the
// function: no paste, and not even the empty-clipboard fallback that starts an
// editing session. The button appeared dead.
//
// This never surfaced because the other clipboard test REPLACES
// navigator.clipboard with a resolving stub, so the failing path had never been
// executed by anything. It was found by walking the UI inside a native host,
// where the real clipboard says no.
func TestPasteSurvivesADeniedClipboard(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got struct {
		Threw   bool   `json:"threw"`
		Error   string `json:"error"`
		Editing bool   `json:"editing"`
	}
	evalJS(t, ctx, `(async () => {
	  // Refuse exactly the way the platform does.
	  Object.defineProperty(navigator, 'clipboard', {
	    configurable: true,
	    value: {
	      readText: () => Promise.reject(new DOMException(
	        'The request is not allowed by the user agent or the platform in the current context.',
	        'NotAllowedError')),
	    },
	  })

	  const out = { threw: false, error: '', editing: false }
	  try {
	    await window.__app.pasteMarkdown()
	  } catch (e) {
	    out.threw = true
	    out.error = String(e && e.message)
	  }
	  await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))

	  // The fallback is to start an editing session rather than leave the user
	  // on an empty state that did not respond.
	  out.editing = !document.body.classList.contains('app-empty')
	  return out
	})()`, &got)

	if got.Threw {
		t.Errorf("a denied clipboard escaped pasteMarkdown as a rejection: %s", got.Error)
	}
	if !got.Editing {
		t.Error("Paste did nothing on a denied clipboard: the empty state is still showing, " +
			"so the button is indistinguishable from a broken one")
	}
}
