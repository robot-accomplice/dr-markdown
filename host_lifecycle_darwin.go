//go:build darwin

package main

import (
	"errors"
	"sync"
)

// Host lifecycle, and the concurrency primitives every goroutine in the host
// derives its exit from.
//
// The host spawns goroutines it does not own the lifetime of: one per bound
// call, one per fire-and-forget dialog. Without a shared exit signal each of
// them can outlive the window — parked on a dialog nobody can answer any more,
// or resolving a promise into a WKWebView that has been torn down. A leaked
// goroutine is invisible until it is the thing holding a process open.

// errHostClosed is returned by any operation abandoned because the host is
// shutting down. Distinguishable from a cancelled context on purpose: one means
// the caller changed its mind, the other means there is no longer a window to
// answer, and only the second is a reason to stop trying.
var errHostClosed = errors.New("host is shutting down")

var (
	hostDoneOnce sync.Once
	// hostDone is closed once, when AppKit tells us the application is
	// terminating. Everything that can block derives its exit from it.
	hostDone = make(chan struct{})
)

// beginShutdown closes hostDone exactly once. The //export wrapper lives in
// host_darwin.go: cgo only processes //export directives in files that
// import "C", and this file deliberately does not.
func beginShutdown() {
	hostDoneOnce.Do(func() { close(hostDone) })
}

// hostClosing reports whether shutdown has begun, without blocking.
func hostClosing() bool {
	select {
	case <-hostDone:
		return true
	default:
		return false
	}
}
