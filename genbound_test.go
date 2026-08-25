package main

import (
	"os"
	"os/exec"
	"testing"
)

// A generated file that is tracked but stale is the defect this guards, and
// this project has already paid for it: the generated binding is
// gitignored, and a stale copy on a build machine once turned SyncDocuments
// into a silent no-op, leaving Go with no documents at all.
//
// Tracking the artefact only helps if something proves it still matches its
// source. Deliberately NOT behind the darwin/ownhost tag — the generator and
// app.go are both platform-independent, so CI can run this on Linux even
// though it never compiles the file being checked.
func TestGeneratedDispatchMatchesAppGo(t *testing.T) {
	const path = "bound_generated.go"

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the generated dispatcher is missing: %v", err)
	}
	t.Cleanup(func() { _ = os.WriteFile(path, before, 0o644) })

	if out, err := exec.Command("go", "run", "./tools/genbound").CombinedOutput(); err != nil {
		t.Fatalf("genbound failed: %v\n%s", err, out)
	}
	// The committed file is gofmt'd, so the comparison has to be too or every
	// run reports a difference that is only whitespace.
	if out, err := exec.Command("gofmt", "-w", path).CombinedOutput(); err != nil {
		t.Fatalf("gofmt failed: %v\n%s", err, out)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the committed dispatcher does not match app.go. " +
			"Run `go run ./tools/genbound && gofmt -w bound_generated.go` and commit the result.")
	}
}

// The generator refuses to bind a method whose panic would reach nobody. That
// refusal is the strongest guard in the chain, so it gets checked rather than
// assumed: every bound method in app.go carries its guard today, and a new one
// arriving without it must fail generation rather than ship unguarded.
func TestGeneratorRefusesAnUnguardedBoundMethod(t *testing.T) {
	src, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile("app.go", src, 0o644) })

	// Remove one guard and confirm generation fails naming that method.
	broken := []byte(replaceFirst(string(src),
		"\tdefer a.reportPanic(\"SetDirty\")\n", ""))
	if string(broken) == string(src) {
		t.Fatal("could not find SetDirty's guard to remove; this test is no longer testing anything")
	}
	if err := os.WriteFile("app.go", broken, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("go", "run", "./tools/genbound").CombinedOutput()
	if err == nil {
		t.Fatal("generation succeeded with an unguarded bound method; the refusal does not work")
	}
	if !contains(string(out), "SetDirty") {
		t.Errorf("the refusal did not name the offending method:\n%s", out)
	}
}

func replaceFirst(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func contains(s, sub string) bool { return indexOf(s, sub) >= 0 }
