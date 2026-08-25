package e2e

import "testing"

// The three list buttons must not draw the same mark.
//
// Task List shared `.bullets::after` verbatim, so Bullet List and Task List
// rendered pixel-identical icons — three round dots each — and the only thing
// separating them was the `title` tooltip. Nothing failed, because no test had
// ever compared one icon against another: each was reachable, labelled and
// clickable, which is all the suite asked.
//
// So this compares the marks to each other rather than checking that each
// exists. It reads `::after`, which is where all three marks live: `.icon-lines
// ::before` draws the three lines every one of them shares.
func TestRibbonListIconsAreDistinctFromEachOther(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var verdict string
	evalJS(t, ctx, `(() => {
		const buttons = {
			'bullet-list': null,
			'numbered-list': null,
			'task-list': null,
		}
		for (const command of Object.keys(buttons)) {
			const el = document.querySelector('[data-command="' + command + '"]')
			if (!el) return 'FAIL: no ribbon button for ' + command
			const cs = getComputedStyle(el, '::after')
			// Every distinguishing mark is drawn by ::after, as an image, a
			// glyph, or a box-shadow. Collapse all three into one fingerprint.
			buttons[command] = [
				cs.content,
				cs.backgroundImage,
				cs.boxShadow,
				cs.borderRadius,
				cs.width + 'x' + cs.height,
			].join(' | ')
		}

		// None may be absent: an empty ::after would make two buttons "differ"
		// only because both are blank.
		for (const [command, mark] of Object.entries(buttons)) {
			if (/^(none|normal)/.test(mark) && !/url|gradient|px/.test(mark)) {
				return 'FAIL: ' + command + ' draws no mark at all: ' + mark
			}
		}

		const pairs = [
			['bullet-list', 'task-list'],
			['bullet-list', 'numbered-list'],
			['numbered-list', 'task-list'],
		]
		for (const [a, b] of pairs) {
			if (buttons[a] === buttons[b]) {
				return 'FAIL: ' + a + ' and ' + b + ' draw the SAME mark:\n  ' + buttons[a]
			}
		}
		return 'ok'
	})()`, &verdict)

	if verdict != "ok" {
		t.Error(verdict)
	}
}
