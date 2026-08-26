package main

import (
	"fmt"
	"runtime/debug"

	"dr-markdown/internal/eventlog"
)

// recordStartupPanic records a panic raised on the main goroutine before the
// application's own guards can see it, then lets it continue.
//
// App.reportPanic covers bound methods and the lifecycle callbacks, all of
// which run only after the App exists and only through paths that defer it.
// Startup is neither: NewApp builds the App, and Run drives the host, and a
// panic in either used to leave nothing — no record, no dialog, and a process
// that simply stopped.
//
// It RE-PANICS rather than swallowing. A failure during startup means the
// application cannot run, and continuing into a half-built state would trade a
// visible crash for an invisible one. The point is the record, not the rescue,
// which is why the crash still looks exactly as it did.
func recordStartupPanic(events *eventlog.Log) {
	recovered := recover()
	if recovered == nil {
		return
	}
	events.Record("panic", map[string]string{
		"operation": "startup",
		"phase":     "before the application was running",
		"message":   fmt.Sprint(recovered),
		"stack":     string(debug.Stack()),
	})
	panic(recovered)
}
