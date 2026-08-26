package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// Changing one code block's language must change THAT block and no other.
//
// This used to be reached through two affordances the app drew on its own
// shell: a right-click menu and a hover "Language" button. Both are gone with
// the shell, because drawing that shell over the editor's own node is what left
// code blocks uneditable (#77). The editor's node view carries a searchable
// language picker of its own, so the capability survives the removal — but the
// targeting guarantee is the part worth defending, and it is defended here.
//
// The app's contextual controls bar remains a separate route to the same
// change, covered by TestContextualDocumentControlsManageBlocksInPlace.
func TestExistingCodeBlockLanguageCanBeChangedFromTheBlockPicker(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{
		"# Code Blocks",
		"",
		"```javascript",
		"const first = 1",
		"```",
		"",
		"```text",
		"second = 2",
		"```",
		"",
	}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	if !waitForJS(t, ctx, `document.querySelectorAll('#wysiwyg .milkdown-code-block .language-button').length === 2`) {
		t.Fatal("both code blocks should expose the editor's language picker")
	}

	// Open the SECOND block's picker and choose Python through it.
	evalJS(t, ctx, `document.querySelectorAll('#wysiwyg .milkdown-code-block .language-button')[1].click(); 'ok'`, &res)

	var picked string
	evalJS(t, ctx, `(async () => {
		const picker = document.querySelectorAll('#wysiwyg .milkdown-code-block .language-picker')[1]
		if (!picker) return '(no picker opened)'
		const search = picker.querySelector('input')
		if (search) {
			search.focus()
			search.value = 'python'
			search.dispatchEvent(new Event('input', { bubbles: true }))
			await new Promise((r) => setTimeout(r, 300))
		}
		const options = Array.from(picker.querySelectorAll('li, .list-item, button, [role="option"]'))
		const python = options.find((o) => o.textContent.trim().toLowerCase() === 'python')
		if (!python) return '(no python option among ' + options.length + ')'
		python.click()
		await new Promise((r) => setTimeout(r, 600))
		return window.__app.getMarkdown()
	})()`, &picked)

	if strings.HasPrefix(picked, "(") {
		t.Fatalf("could not drive the editor's language picker: %s", picked)
	}

	// The first block must be untouched, and its body must not have moved.
	if !strings.Contains(picked, "```javascript\nconst first = 1") {
		t.Errorf("changing the second block's language altered the first:\n%s", picked)
	}
	// The second block must now be Python, and must still hold its own body.
	if !strings.Contains(strings.ToLower(picked), "```python\nsecond = 2") {
		t.Errorf("the second block's language was not changed to python:\n%s", picked)
	}

	// The fence carries the NORMALIZED language, not the picker's display name.
	//
	// #78: the node view writes whatever the picker hands it, and the picker
	// supplies CodeMirror's display name, so this produced "```Python" where
	// every other route in this app writes "```python". Markdown treats info
	// strings case-insensitively so nothing rendered differently, but this
	// project holds a byte-identical round-trip corpus, and a file gaining a
	// capitalised fence purely because of which control the user reached for is
	// an inconsistency the corpus would have to encode.
	if strings.Contains(picked, "```Python") {
		t.Errorf("the picker wrote the display name into the fence:\n%s", picked)
	}
	if !strings.Contains(picked, "```python\nsecond = 2") {
		t.Errorf("expected a normalized fence language from the picker:\n%s", picked)
	}
}

// A fence the USER authored capitalised must come back capitalised.
//
// This is the other half of #78, and the reason the fix is at the picker rather
// than on serialize. Normalizing on the way out would tidy this document too,
// which the fidelity survey would correctly fail: a document must come back as
// it went in, whatever the app would have written itself.
func TestAUserAuthoredFenceKeepsItsOwnCasing(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	fixture := strings.Join([]string{
		"```Python",
		"already = 'capitalised'",
		"```",
		"",
		"```JavaScript",
		"const shouty = true",
		"```",
		"",
	}, "\n")
	var res string
	evalJS(t, ctx, "window.__app.setMarkdown("+strconv.Quote(fixture)+").then(() => 'ok')", &res)

	var out string
	evalJS(t, ctx, `(async () => {
		await new Promise((r) => setTimeout(r, 600))
		return window.__app.getEditorMarkdown()
	})()`, &out)

	for _, fence := range []string{"```Python", "```JavaScript"} {
		if !strings.Contains(out, fence) {
			t.Errorf("the document was opened with %s and came back without it — normalizing "+
				"the user's own fence is a fidelity regression, not a fix:\n%s", fence, out)
		}
	}
}
