package main

import (
	"fmt"
	"os"
)

const outPath = "bound_generated.go"

func main() {
	src, err := os.ReadFile("app.go")
	if err != nil {
		fail("read app.go: %v", err)
	}
	methods, err := ParseApp(string(src))
	if err != nil {
		fail("%v", err)
	}
	imports, err := ParseImports(string(src))
	if err != nil {
		fail("%v", err)
	}
	if len(methods) == 0 {
		fail("no exported App methods found; the parser is broken, not the code")
	}

	// A method bound to the frontend without its panic guard reaches nobody when
	// it fails. Refuse to generate rather than emit a binding for it.
	var unguarded []string
	for _, m := range methods {
		if !m.Guarded {
			unguarded = append(unguarded, m.Name)
		}
	}
	if len(unguarded) > 0 {
		fail("these bound methods lack `defer a.reportPanic(\"Name\")` as their first statement:\n  %v", unguarded)
	}

	if err := os.WriteFile(outPath, []byte(EmitDispatch(methods, imports)), 0o644); err != nil {
		fail("%v", err)
	}
	fmt.Printf("wrote %s (%d methods)\n", outPath, len(methods))
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "genbound: "+format+"\n", args...)
	os.Exit(1)
}
