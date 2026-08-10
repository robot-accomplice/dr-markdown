package main

import (
	"context"
	"io/fs"
)

// hostPort is what the application needs from whatever runs it: a window, a way
// to serve its assets, lifecycle callbacks, and the native operations.
//
// Defined from the application's needs, never from what a particular host
// offers. A boundary drawn around the shape of the current host encodes that
// host, and makes every later replacement expensive for a reason we created
// rather than one that is real.
//
// It composes nativePort rather than replacing it: those eleven operations
// already have a tested shape and a working fake, and re-cutting them would
// risk behaviour for no gain.
//
// Everything here is an operating-system concern and survives any answer to the
// user-interface question — including drawing the interface directly instead of
// hosting a webview. See docs/superpowers/specs/2026-08-10-host-boundary-design.md.
type hostPort interface {
	Native() nativePort
	Run(cfg hostConfig) error
}

// hostConfig is the application's description of the window it wants. It names
// no host type, so a second implementation needs no change here.
type hostConfig struct {
	Title  string
	Width  int
	Height int
	// Assets is the embedded frontend. How it reaches the view is the host's
	// business: Wails serves it through a custom URL scheme handler in-process,
	// and nothing here assumes that.
	Assets        fs.FS
	OnStartup     func(context.Context)
	OnBeforeClose func(context.Context) bool
	// OnFileOpen receives a path the OS routed to the app. At launch this fires
	// before the view exists, which is why App holds the path until the frontend
	// asks for it rather than being told (#53).
	OnFileOpen func(path string)
	Bind       []interface{}
}
