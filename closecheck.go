package main

import "fmt"

// The close-guard check's judgement, separated from the host so it can be
// tested without one.
//
// GitHub #100: reportCloseDecision printed VERDICT: PASS on BOTH guard
// outcomes. It did not know which mode it was running in, so under -close-dirty
// a guard that ALLOWED the close — unsaved work dropped silently, the exact
// failure the mode exists to catch — was reported as a passing clean close,
// with the words "a clean document closed without a prompt" against a document
// that was dirty and had prompted.
//
// It survived because the mode needs a person to answer its dialog, so nothing
// ran it unattended and nothing asserted what it should say. Splitting the
// judgement out fixes that: driving the window still needs a person, but
// whether a given observation is a pass is a pure function, and the unit tests
// beside this file exercise every branch — including the ones that must FAIL.
//
// A dirty close has no single correct outcome, which is why the original could
// not simply compare a boolean. It depends on the button: Cancel must prevent
// the close, Don't Save must allow it, and Save must allow it once the save
// succeeds. The pairing is the assertion.

// closeObservation is what the harness saw while the guard ran.
type closeObservation struct {
	// dirty is the mode: false for -close, true for -close-dirty.
	dirty bool
	// prevented is what the guard returned.
	prevented bool
	// prompts counts dialogs raised while the guard ran.
	prompts int
	// answer is the button chosen, empty if no dialog was answered.
	answer string
}

// judgeClose reports whether the observation is correct, and why.
//
// The message is the point. A verdict that cannot say what it expected is only
// marginally better than one that always passes.
func judgeClose(o closeObservation) (ok bool, why string) {
	if !o.dirty {
		switch {
		case o.prompts > 0:
			return false, fmt.Sprintf("a clean document raised %d prompt(s): "+
				"there is nothing to confirm and the close should be silent", o.prompts)
		case o.prevented:
			return false, "the guard PREVENTED a clean document from closing"
		}
		return true, "a clean document closed silently"
	}

	if o.prompts == 0 {
		return false, "a dirty document raised NO prompt and the guard " +
			verb(o.prevented) + " the close: unsaved work would be lost silently"
	}

	switch o.answer {
	case "Cancel", "":
		// Empty means the dialog failed or was dismissed without a choice. Both
		// must protect the work: promptUnsaved returns prevent on a dialog error
		// for exactly this reason.
		if !o.prevented {
			return false, "the dialog was cancelled or failed, and the guard ALLOWED " +
				"the close anyway: unsaved work would be lost"
		}
		return true, "Cancel prevented the close"
	case "Don't Save":
		if o.prevented {
			return false, "Don't Save was chosen and the guard PREVENTED the close: " +
				"the user asked to discard and the window stayed open"
		}
		return true, "Don't Save allowed the close"
	case "Save":
		if o.prevented {
			return false, "Save was chosen and the guard PREVENTED the close: " +
				"the save did not complete, so the window was held open"
		}
		return true, "Save completed and allowed the close"
	}
	return false, fmt.Sprintf("the dialog reported an unknown answer %q, so what the guard "+
		"did with it cannot be judged", o.answer)
}

func verb(prevented bool) string {
	if prevented {
		return "PREVENTED"
	}
	return "ALLOWED"
}
