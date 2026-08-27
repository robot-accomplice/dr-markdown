package e2e

import "testing"

// Formatted, Raw and Split are one control, and one choice.
//
// Reported from real use: the mode selection was two controls where it is one
// decision — a two-way segmented pair, a divider, and a separate Split toggle
// beside it. The model was already a three-way choice: setMode takes one of
// wysiwyg, raw or split, and the application holds a single state.mode. Only the
// control disagreed.
//
// It also raised a question the application could not answer. toggleSplit
// returned to Formatted from Split regardless of whether you had been in Raw
// when you turned it on, so Split behaved as a mode while being presented as a
// modifier on one.
func TestTheThreeModesAreOneControl(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var shape struct {
		Group    string   `json:"group"`
		Buttons  []string `json:"buttons"`
		Roles    []string `json:"roles"`
		Siblings int      `json:"siblings"`
	}
	evalJS(t, ctx, `(() => {
		const group = document.querySelector('.mode-controls')
		const sw = document.querySelector('.mode-switch')
		const bs = Array.from(sw ? sw.querySelectorAll('button') : [])
		return {
			group: group ? group.getAttribute('role') : '(none)',
			buttons: bs.map((b) => b.textContent.trim()),
			roles: bs.map((b) => b.getAttribute('role')),
			// Anything in the group that is NOT inside the switch is a control
			// sitting beside the choice rather than being part of it.
			siblings: group ? group.querySelectorAll(':scope > *:not(.mode-switch)').length : -1,
		}
	})()`, &shape)

	t.Logf("group role=%s  buttons=%v  roles=%v  strays=%d",
		shape.Group, shape.Buttons, shape.Roles, shape.Siblings)

	if len(shape.Buttons) != 3 {
		t.Fatalf("the mode switch holds %d controls, want 3: %v", len(shape.Buttons), shape.Buttons)
	}
	if shape.Siblings != 0 {
		t.Errorf("%d control(s) sit beside the mode switch rather than inside it: that is what "+
			"made one choice look like two", shape.Siblings)
	}
	if shape.Group != "radiogroup" {
		t.Errorf("the mode controls expose role=%q, want radiogroup: three exclusive choices "+
			"are radios, not unrelated buttons", shape.Group)
	}
	for i, r := range shape.Roles {
		if r != "radio" {
			t.Errorf("%q exposes role=%q, want radio", shape.Buttons[i], r)
		}
	}
}

// Split selects. It does not toggle back.
//
// The old behaviour returned to Formatted whichever mode you came from, which is
// what made it read as a modifier. A peer selects itself and stays selected.
func TestSplitSelectsRatherThanToggles(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var modes []string
	evalJS(t, ctx, `(async () => {
		const out = []
		const click = async (id) => {
			document.getElementById(id).click()
			await new Promise((r) => setTimeout(r, 700))
			out.push(window.__app.state.mode)
		}
		await click('btn-mode-raw')
		await click('btn-split')
		await click('btn-split')   // clicking the selected peer must not leave it
		await click('btn-mode-formatted')
		return out
	})()`, &modes)

	t.Logf("raw -> split -> split -> formatted gave %v", modes)
	want := []string{"raw", "split", "split", "wysiwyg"}
	if len(modes) != len(want) {
		t.Fatalf("expected %d transitions, got %v", len(want), modes)
	}
	for i := range want {
		if modes[i] != want[i] {
			t.Errorf("step %d: mode is %q, want %q. Clicking a selected peer must not "+
				"deselect it, and Split must not return to Formatted from Raw.",
				i+1, modes[i], want[i])
		}
	}
}
