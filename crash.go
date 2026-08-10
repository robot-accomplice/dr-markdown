package main

import (
	"fmt"
	"runtime/debug"
)

// reportPanic records a panic, tells the user, and then lets it keep travelling.
// Use it as the first deferred call in an operation: defer a.reportPanic("SaveDocument").
//
// It does not stop the panic, and that is the point. Wails recovers panics in
// bound method dispatch already (internal/frontend/dispatcher: ProcessMessage
// defers a recover unless DisablePanicRecovery is set, and this app does not set
// it), so the process survives one either way. What it did not do was leave
// anything a user could report: the dispatcher logs through the Wails logger,
// which in a packaged build writes to a stream nobody reads, and the event trail
// beside the preference store saw nothing at all.
//
// Recovering here instead of re-panicking would be worse than the silence. The
// deferred call runs during unwinding, so the bound method would return its zero
// values — SaveDocument would hand the frontend a nil error, and the frontend
// would report a save that never touched the disk.
//
// Like the event log itself, this is diagnostic: it must never be the thing that
// breaks. A missing dependency is skipped rather than dereferenced, because a
// panic raised inside a panic handler replaces a recorded failure with an
// unrecorded one.
func (a *App) reportPanic(operation string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	a.events.Record("panic", map[string]string{
		"operation": operation,
		"message":   fmt.Sprint(recovered),
		"stack":     string(debug.Stack()),
	})
	// Every panic is recorded; only the first raises a dialog. UpdateContent is
	// pushed debounced on every edit, so a panic there repeats for as long as the
	// user types, and a blocking native dialog per tick would leave the app
	// unusable behind a modal storm — worse than the silently dead call this
	// replaced. One per session rather than one per operation, because the
	// instruction is "restart" and that does not improve on repetition.
	if a.native != nil && a.ctx != nil {
		a.panicDialog.Do(func() {
			a.showPanicDialog(operation)
		})
	}
	panic(recovered)
}

func (a *App) showPanicDialog(operation string) {
	a.native.ShowError(a.ctx, "Dr. Markdown hit an internal error", fmt.Sprintf(
		"An internal error interrupted %s, and the app may no longer be in a state it understands.\n\n"+
			"Copy any unsaved text into another app, then restart Dr. Markdown.\n\n"+
			"The details were written to the event log beside your preferences.",
		operation))
}
