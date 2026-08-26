package main

import (
	"strings"
	"testing"
)

// The close check must be able to FAIL.
//
// GitHub #100: it printed PASS on both guard outcomes, so under -close-dirty a
// guard that dropped unsaved work reported success. These cases are mostly the
// failures, because a check is only worth its passing cases if its failing ones
// are real.
func TestJudgeClose(t *testing.T) {
	for _, c := range []struct {
		name string
		obs  closeObservation
		want bool
	}{
		// Clean: nothing to confirm, so nothing should appear and nothing should
		// hold the window open.
		{"clean closes silently", closeObservation{dirty: false}, true},
		{"clean is prevented", closeObservation{dirty: false, prevented: true}, false},
		{"clean raises a prompt", closeObservation{dirty: false, prompts: 1, answer: "Cancel", prevented: true}, false},

		// The defect this issue is about: a dirty document that closes with no
		// prompt at all. Under the old code this printed
		// "PASS — a clean document closed without a prompt".
		{"dirty closes with NO prompt", closeObservation{dirty: true}, false},
		{"dirty prevented with no prompt", closeObservation{dirty: true, prevented: true}, false},

		// Dirty: the button decides, so the pairing is what is asserted.
		{"Cancel prevents", closeObservation{dirty: true, prompts: 1, answer: "Cancel", prevented: true}, true},
		{"Cancel allows anyway", closeObservation{dirty: true, prompts: 1, answer: "Cancel"}, false},
		{"Don't Save allows", closeObservation{dirty: true, prompts: 1, answer: "Don't Save"}, true},
		{"Don't Save prevented", closeObservation{dirty: true, prompts: 1, answer: "Don't Save", prevented: true}, false},
		{"Save allows", closeObservation{dirty: true, prompts: 1, answer: "Save"}, true},
		{"Save prevented (save failed)", closeObservation{dirty: true, prompts: 1, answer: "Save", prevented: true}, false},

		// A dialog that failed reports no answer. promptUnsaved returns prevent
		// in that case so the work survives; allowing the close would not.
		{"dialog failed, work protected", closeObservation{dirty: true, prompts: 1, prevented: true}, true},
		{"dialog failed, close allowed", closeObservation{dirty: true, prompts: 1}, false},

		{"unknown answer", closeObservation{dirty: true, prompts: 1, answer: "Maybe"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, why := judgeClose(c.obs)
			if got != c.want {
				t.Errorf("judgeClose(%+v) = %v, want %v\n  said: %s", c.obs, got, c.want, why)
			}
			if why == "" {
				t.Error("a verdict with no reason is the defect this replaced")
			}
		})
	}
}

// The old verdict's exact words, asserted as absent.
//
// It reported "a clean document closed without a prompt" for a DIRTY document
// that had prompted. No judgement may describe a dirty observation as clean.
func TestNoVerdictCallsADirtyDocumentClean(t *testing.T) {
	for _, obs := range []closeObservation{
		{dirty: true},
		{dirty: true, prompts: 1, answer: "Don't Save"},
		{dirty: true, prompts: 1, answer: "Cancel", prevented: true},
		{dirty: true, prompts: 1, prevented: true},
	} {
		if _, why := judgeClose(obs); strings.Contains(why, "clean") {
			t.Errorf("judgeClose(%+v) described a dirty document as clean: %q", obs, why)
		}
	}
}
