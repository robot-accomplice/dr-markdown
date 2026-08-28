package main

import (
	"embed"
	"os"
	"runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// AppKit requires its event loop on the process's first thread. Without
	// this the window may come up on an arbitrary OS thread and behave in ways
	// that are undefined rather than merely wrong.
	runtime.LockOSThread()
}

func main() {
	if runHarness() {
		return
	}

	// The trail is opened FIRST, and a panic on the way up is recorded against
	// it. Measured before this existed: a panic inside NewApp left no file at
	// all, and a panic after NewApp but before Run left nothing either — even
	// though the log existed by then — because nothing in main recovered. The
	// boundary was never the App's existence, it was whether a call happens to
	// be wrapped by reportPanic, and main is not (#62).
	//
	// This covers the MAIN goroutine only. A panic on another goroutine cannot
	// be recovered from here; that is Go, not an oversight, and it is said out
	// loud so this is not later mistaken for total coverage.
	events := NewEventLog()
	defer recordStartupPanic(events)

	host := newHost()
	app := NewApp(host.Native(), events)

	err := host.Run(hostConfig{
		Title:         "Dr Markdown",
		Width:         1440,
		Height:        900,
		Assets:        assets,
		OnStartup:     app.startup,
		OnBeforeClose: app.beforeClose,
		OnFileOpen:    app.openFileFromOS,
		// A navigation the host refuses is a security decision taken against
		// untrusted document content without asking anyone, so it is recorded
		// rather than only prevented.
		OnNavigationBlocked: app.recordBlockedNavigation,
		Bind:                []any{app},
	})
	if err != nil {
		println("Error:", err.Error())
		os.Exit(1)
	}
}
