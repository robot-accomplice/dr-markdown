//go:build darwin

package main

/*
#include <stdlib.h>
void hostRunBare(void);
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"
)

// The modal operations cannot be driven by the automated gates: NSAlert,
// NSOpenPanel and NSSavePanel are modal and only a person can answer them.
// Rather than leave them unverified, this walks an operator through each one
// and checks the answer that came back.
//
// What it is actually testing is not "does a dialog appear" — it is the two
// things that are easy to get wrong and silent when wrong:
//
//   - BUTTON MAPPING. hostDialog converts NSModalResponse to an index and
//     returns the button's TITLE, because every call site compares against
//     "Save", "Overwrite" and so on. An off-by-one maps "Don't Save" onto
//     "Save" and silently discards a user's work. The middle button of three
//     is where that shows up, so the script asks for it deliberately.
//   - CANCEL SEMANTICS. A cancelled dialog must return "" and a nil error.
//     Treating cancel as an error would make dismissing a save dialog look
//     like a failed save.
type modalStep struct {
	name    string
	prompt  string
	run     func() string
	want    string // empty means "any non-empty answer"
	wantAny bool
}

func modalSteps(native darwinNative, ctx context.Context) []modalStep {
	return []modalStep{
		{
			name:   "error alert",
			prompt: "Click OK.",
			run: func() string {
				s, _ := showDialog(ctx, dialog{title: "Spike Check", message: "This is the error dialog.",
					buttons: "OK", defaultButton: "OK", cancelButton: "OK", isError: true})
				return s
			},
			want: "OK",
		},
		{
			name:   "unsaved changes, middle button",
			prompt: "Three buttons. CLICK \"Don't Save\".",
			run:    func() string { s, _ := native.ConfirmUnsaved(ctx); return s },
			want:   "Don't Save",
		},
		{
			name:   "overwrite prompt, RETURN key",
			prompt: "Press RETURN. Do not click anything.",
			run:    func() string { s, _ := native.ConfirmOverwriteChanged(ctx, "/tmp/notes.md"); return s },
			want:   "Cancel",
		},
		{
			name:   "overwrite prompt, ESCAPE key",
			prompt: "Press ESCAPE. Do not click anything.",
			run:    func() string { s, _ := native.ConfirmOverwriteChanged(ctx, "/tmp/notes.md"); return s },
			want:   "Cancel",
		},
		{
			name:   "overwrite prompt, deliberate click",
			prompt: "CLICK \"Overwrite\".",
			run:    func() string { s, _ := native.ConfirmOverwriteChanged(ctx, "/tmp/notes.md"); return s },
			want:   "Overwrite",
		},
		{
			name:   "open panel, cancelled",
			prompt: "Click Cancel. Do not choose a file.",
			run:    func() string { s, _ := native.OpenMarkdownFile(ctx); return s },
			want:   "",
		},
		{
			name:    "open panel, file chosen",
			prompt:  "Choose any .md file and click Open.",
			run:     func() string { s, _ := native.OpenMarkdownFile(ctx); return s },
			wantAny: true,
		},
		{
			name:   "save panel, cancelled",
			prompt: "Click Cancel.",
			run:    func() string { s, _ := native.SaveMarkdownFile(ctx, "untitled.md"); return s },
			want:   "",
		},
	}
}

// runOneModalCheck runs a SINGLE step: one instruction, one dialog, one result.
//
// The batch version asked an operator to hold eight instructions in their head
// while answering eight dialogs, three of which look identical and want
// different answers. It produced results that could not be read as evidence
// about the code, which makes it worse than no test.
func runOneModalCheck(index int) {
	native := darwinNative{}
	ctx := context.Background()
	steps := modalSteps(native, ctx)

	if index < 1 || index > len(steps) {
		fmt.Printf("step %d does not exist; there are %d\n", index, len(steps))
		os.Exit(2)
	}
	step := steps[index-1]

	fmt.Printf("STEP %d/%d  %s\n%s\n\n", index, len(steps), step.name, step.prompt)

	got := step.run()

	switch {
	case step.wantAny && got == "":
		fmt.Printf("FAIL — expected a path, got \"\"\n")
		os.Exit(1)
	case step.wantAny:
		if _, err := os.Stat(got); err != nil {
			fmt.Printf("FAIL — returned %q but it does not exist: %v\n", got, err)
			os.Exit(1)
		}
		fmt.Printf("PASS — %q (exists)\n", got)
	case got != step.want:
		fmt.Printf("FAIL — expected %q, got %q\n", step.want, got)
		os.Exit(1)
	default:
		fmt.Printf("PASS — %q\n", got)
	}
	os.Exit(0)
}

// parseStep reads the step number given to -modal.
func parseStep(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func runModalCheckMode(step int) {
	go func() {
		// Let the run loop come up before the first dispatch to it.
		time.Sleep(300 * time.Millisecond)
		runOneModalCheck(step)
	}()
	C.hostRunBare()
}
