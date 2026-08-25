package main

import (
	"fmt"
	"runtime/debug"
)

// reportPanic records a panic, tells the user, and then lets it keep travelling.
// Use it as the first deferred call in an operation: defer a.reportPanic("SaveDocument").
//
// It does not stop the panic, and that is the point. The host recovers panics in
// bound-method dispatch and REJECTS the frontend's promise, so the process
// survives one and the caller is told. What a recovery on its own does not do is
// leave anything a user could report: in a packaged build a log line goes to a
// stream nobody reads, and the event trail beside the preference store would see
// nothing at all.
//
// This described the previous framework's dispatcher until v0.6.0, and the
// behaviour it described
// was worse: that recovery returned an EMPTY result, the runtime's Callback threw
// on JSON.parse("") before reaching the pending callback, and no call timeout was
// ever armed — so the frontend's await never settled (#61). The host this project
// owns rejects instead, which the -gates mode proves on every run.
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
