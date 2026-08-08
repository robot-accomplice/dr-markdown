package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// FIDELITY CHARACTERIZATION.
//
// The WYSIWYG surface parses markdown into the vendored editor's document model
// and re-serializes it, so any construct the model cannot express is rewritten
// or lost — and because an edit replaces the whole buffer, the blast radius of
// one keystroke is the whole file. This test does not assert that behaviour is
// correct. It RECORDS exactly which constructs are affected, so the README's
// caution is derived from measured behaviour rather than from memory, and so a
// future editor change that fixes or worsens any line here shows up as a
// failing test instead of passing unnoticed.
//
// Every `want` below was observed, not predicted.
func TestWysiwygRewritesTheseConstructs(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		// `<br>` deletion was FIXED — see protectBreaks in editor.js. What
		// remains is table column reflow, which widens padding to fit the
		// sentinel while the cell CONTENT is preserved exactly.
		{
			"table padding reflows around a preserved br",
			"| h | v |\n| --- | --- |\n| x<br>y | z |\n",
			"| h        | v |\n| -------- | - |\n| x<br>y | z |\n",
		},
		// The EDITOR still normalizes CRLF; the FILE no longer loses it. Line
		// endings are restored at the document boundary, which is why this case
		// still records LF here while TestLineEndingsSurviveAnEditAndSave proves
		// a CRLF file saves as CRLF. Keeping both makes the distinction visible.
		{"editor normalizes crlf internally", "line one\r\nline two\r\n", "line one\nline two\n"},
		{"hyphen bullets become asterisks", "- a\n- b\n", "* a\n* b\n"},
		{"plus bullets become asterisks", "+ a\n+ b\n", "* a\n* b\n"},
		{"setext heading becomes atx", "Title\n=====\n", "# Title\n"},
		{"closing hashes are stripped", "# Heading ##\n", "# Heading\n"},
		{"tilde fence becomes backtick fence", "~~~\ncode\n~~~\n", "```\ncode\n```\n"},
		{"thematic break is respelled", "a\n\n---\n\nb\n", "a\n\n***\n\nb\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			var got string
			evalJS(t, ctx,
				"window.__app.setMarkdown("+string(in)+").then(() => window.__app.getEditorMarkdown())",
				&got)
			if got != tc.want {
				t.Errorf("recorded behaviour changed.\n in:   %q\n want: %q\n got:  %q\n"+
					"If this is an improvement, update the README caution too.", tc.in, tc.want, got)
			}
		})
	}
}

// Raw mode is the answer the README gives users whose bytes matter, so what it
// does needs a test rather than an assumption.
//
// Measured: it preserves every CONSTRUCT the WYSIWYG surface rewrites — inline
// <br>, reference definitions (including unused ones), mixed bullets, closing
// hashes, two-space hard breaks — but it does NOT preserve CRLF line endings.
// A textarea normalizes them to LF on input per the HTML spec, so a
// Windows-authored file still comes back whole-file changed. The README says
// exactly this rather than calling Raw mode byte-exact.
func TestRawModeEditsPreserveEveryOtherByte(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	original := "# Title ##\r\n\r\nCell one<br>cell two.  \r\n\r\n- a\r\n+ b\r\n\r\n" +
		"See the [spec][s].\r\n\r\n[s]: https://example.com/spec\r\n[unused]: https://example.com/x\r\n"
	edited := original + "\r\nappended by the user\r\n"

	in, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	next, err := json.Marshal(edited)
	if err != nil {
		t.Fatal(err)
	}

	var got string
	evalJS(t, ctx, `(async () => {
		await window.__app.setMode('raw')
		await window.__app.setMarkdown(`+string(in)+`)
		window.__app.debugReplaceRaw(`+string(next)+`)
		return window.__app.getMarkdown()
	})()`, &got)

	// The one recorded exception: line endings are normalized to LF.
	want := strings.ReplaceAll(edited, "\r\n", "\n")
	if got != want {
		t.Errorf("raw mode altered content beyond the recorded line-ending normalization.\n want: %q\n got:  %q", want, got)
	}
	if strings.Contains(got, "<br>") == false ||
		strings.Contains(got, "[unused]: https://example.com/x") == false ||
		strings.Contains(got, "+ b") == false ||
		strings.Contains(got, "# Title ##") == false {
		t.Errorf("raw mode lost a construct the README promises it keeps: %q", got)
	}
}

// A Windows-authored file must come back with its line endings intact. Nothing
// preserved them, so every line of every CRLF file changed on save and a
// one-word edit produced a whole-file diff — the single loudest complaint for
// anyone keeping notes in version control.
//
// This belongs at the document boundary rather than inside the WYSIWYG adapter:
// the raw editor normalizes CRLF too (a textarea does that per the HTML spec),
// so fixing it at the editor would leave the same bug on the other surface.
func TestLineEndingsSurviveAnEditAndSave(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	original := "# Title\r\n\r\nFirst line.\r\nSecond line.\r\n"
	in, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var saved string
	evalJS(t, ctx, `(async () => {
		globalThis.__saved = null
		globalThis.go = { main: { App: {
			LoadPreferences: async () => ({settings:{},rawOptions:{},recents:[]}),
			OpenDocument: async () => ({ path: '/tmp/crlf.md', content: `+string(in)+` }),
			SaveDocument: async (p, c) => { globalThis.__saved = c },
			ResolveUnsavedChanges: async () => true,
			SyncDocuments: async () => {}, SetDirty: async () => {}, UpdateContent: async () => {}
		} } }
		await window.__app.openDocument()
		// A real keystroke: the editor re-serializes and that result becomes the
		// buffer. Saving without editing preserves CRLF trivially, because the
		// bytes read from disk are still the ones held in memory — so a test
		// that skipped this step would pass while the bug was fully present.
		window.__app.debugSimulateEdit(window.__app.getEditorMarkdown())
		await window.__app.save()
		return globalThis.__saved
	})()`, &saved)

	if !strings.Contains(saved, "\r\n") {
		t.Errorf("CRLF line endings were rewritten to LF on save: %q", saved)
	}
	if saved != original {
		t.Errorf("saved bytes differ from the file that was opened:\n want %q\n got  %q", original, saved)
	}
}
