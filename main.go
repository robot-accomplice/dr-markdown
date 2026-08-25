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

	host := newHost()
	app := NewApp(host.Native())

	err := host.Run(hostConfig{
		Title:         "Dr. Markdown",
		Width:         1440,
		Height:        900,
		Assets:        assets,
		OnStartup:     app.startup,
		OnBeforeClose: app.beforeClose,
		OnFileOpen:    app.openFileFromOS,
		Bind:          []any{app},
	})
	if err != nil {
		println("Error:", err.Error())
		os.Exit(1)
	}
}
