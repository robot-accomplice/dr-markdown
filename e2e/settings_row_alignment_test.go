package e2e

import "testing"

// A row's control lines up with the label that names it.
//
// Reported from real use, against the Code Block dialog: "the alignment
// between labels and fields here is off".
//
// Every `.settings-row` is a two-line label — a title and a description —
// beside a control, laid out with `align-items: center`. Centering aligns the
// control against the MIDDLE of the whole label group, so the title and the
// control's own text land on different lines. The reader pairs "Language" with
// "Go"; the layout pairs the control with a point in the gap below "Language".
//
// The description is a subordinate line and should not pull the control down.
// The title and the control are the pair, so their text should sit on one line
// regardless of how many lines of description follow.
//
// Measured as the gap between the vertical centre of the title's line box and
// the vertical centre of the control. TOLERANCE is not zero because a select's
// text is centred in a taller box and rounding differs by a fraction of a
// pixel; anything a reader could see is well outside it.
func TestARowsControlLinesUpWithItsTitle(t *testing.T) {
	const tolerance = 2.0

	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	// Open the Code Block dialog through the app, not by injecting markup: the
	// defect is in the shipped layout, and building a fake row would measure
	// the test's own CSS assumptions instead.
	var opened string
	evalJS(t, ctx, `(async () => {
		window.__app.activateRibbonTab('format')
		await new Promise((r) => setTimeout(r, 120))
		const button = document.querySelector('[data-command="code-block"]')
		if (!button) return 'no code-block button'
		button.click()
		await new Promise((r) => setTimeout(r, 250))
		return document.querySelector('[data-code-assistant] .settings-row') ? 'open' : 'missing'
	})()`, &opened)
	if opened != "open" {
		t.Fatalf("could not open the Code Block dialog: %s", opened)
	}

	type row struct {
		Title   string  `json:"title"`
		Offset  float64 `json:"offset"`
		Control string  `json:"control"`
	}
	var rows []row
	evalJS(t, ctx, `(() => {
		const centre = (el) => { const r = el.getBoundingClientRect(); return r.top + r.height / 2 }
		return Array.from(document.querySelectorAll('.settings-row')).map((row) => {
			const strong = row.querySelector('strong')
			// The title's own line box, not the label group's: a Range around the
			// text reports where the line actually sits.
			const range = document.createRange()
			range.selectNodeContents(strong)
			const line = range.getBoundingClientRect()
			const control = row.querySelector('select, input, .settings-segmented, .settings-slider')
			if (!strong || !control) return null
			return {
				title: strong.textContent,
				control: control.tagName.toLowerCase() + (control.className ? '.' + control.className : ''),
				offset: Math.round((centre(control) - (line.top + line.height / 2)) * 10) / 10,
			}
		}).filter(Boolean)
	})()`, &rows)

	if len(rows) == 0 {
		t.Fatal("found no settings rows to measure")
	}

	// The dialog is one row with one select. The Settings panel carries the
	// other control types the same rule now governs — toggle, slider and
	// segmented — and the fix changed all of them, so measuring only the
	// reported row would leave the rest unverified.
	var settingsRows []row
	evalJS(t, ctx, `(async () => {
		document.querySelector('[data-code-action="cancel"]').click()
		window.__app.openSettings()
		await new Promise((r) => setTimeout(r, 300))
		return 'ok'
	})()`, &opened)
	for _, section := range []string{"editor", "appearance"} {
		var got []row
		evalJS(t, ctx, `(async () => {
			const tab = document.querySelector('[data-settings-nav="`+section+`"]')
			if (tab) { tab.click(); await new Promise((r) => setTimeout(r, 250)) }
			const centre = (el) => { const r = el.getBoundingClientRect(); return r.top + r.height / 2 }
			return Array.from(document.querySelectorAll('.settings-panel .settings-row, .settings-scrim .settings-row')).map((row) => {
				const strong = row.querySelector('strong')
				const control = row.querySelector('select, .settings-toggle, .settings-segmented, .settings-slider')
				if (!strong || !control) return null
				const range = document.createRange()
				range.selectNodeContents(strong)
				const line = range.getBoundingClientRect()
				return {
					title: strong.textContent,
					control: control.tagName.toLowerCase() + (control.className ? '.' + control.className : ''),
					offset: Math.round((centre(control) - (line.top + line.height / 2)) * 10) / 10,
				}
			}).filter(Boolean)
		})()`, &got)
		settingsRows = append(settingsRows, got...)
	}
	if len(settingsRows) == 0 {
		t.Error("measured no Settings rows: the other control types went unverified")
	}
	rows = append(rows, settingsRows...)

	for _, r := range rows {
		t.Logf("%-28s %-24s control centre is %+.1fpx from the title's line", r.Title, r.Control, r.Offset)
	}
	// Centring alone does not prove the control is intact. The first attempt at
	// this fix gave every control the band as a min-height, which STRETCHED the
	// toggle from a 20px pill into a 32px one — and a stretched pill still
	// centres, so the offset check above passed it. Its size is the assertion
	// that catches that.
	var toggle struct {
		Height float64 `json:"height"`
		Width  float64 `json:"width"`
	}
	evalJS(t, ctx, `(() => {
		const el = document.querySelector('.settings-toggle')
		const r = el.getBoundingClientRect()
		return { height: Math.round(r.height * 10) / 10, width: Math.round(r.width * 10) / 10 }
	})()`, &toggle)
	t.Logf("toggle pill: %.1f x %.1f", toggle.Width, toggle.Height)
	if toggle.Height != 20 {
		t.Errorf("the toggle pill is %.1fpx tall but is designed as 20px: the band stretched it "+
			"instead of centring it in the band", toggle.Height)
	}

	for _, r := range rows {
		if r.Offset > tolerance || r.Offset < -tolerance {
			t.Errorf("row %q: its %s sits %+.1fpx off the title's line, so %q and the control's "+
				"own text do not read as a pair", r.Title, r.Control, r.Offset, r.Title)
		}
	}
}
