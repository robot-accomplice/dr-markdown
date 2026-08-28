//go:build darwin

package main

import "os"

// newHost returns the macOS host. There is exactly one, and this is the only
// line the application needs to change to run on a different one.
func newHost() hostPort { return darwinHost{} }

// runHarness handles the verification modes and reports whether it took over.
//
// These drive a native window, which chromedp cannot see, so they are the only
// way to exercise the host at all. They are flags rather than a separate binary
// so that what runs is the SAME build a user gets — a harness that tests a
// different binary tests a different program.
func runHarness() bool {
	for i, arg := range os.Args[1:] {
		switch arg {
		case "-drop":
			dropWaitMode = true
		case "-gates":
			gateMode = true
		case "-walk":
			walkMode = true
		case "-close":
			closeCheckMode = true
		case "-nav":
			// Drives a real main-frame navigation off the app's scheme and
			// checks the host refuses it. Headless: no dialog, no human.
			navCheckMode = true
		case "-quit":
			// The CLEAN quit. Verifiable without a human, and it is what proves
			// the terminate: path reaches the guard and that the reply gets back
			// to AppKit at all — the dirty case below cannot run unattended
			// because it raises the save dialog on purpose.
			quitCheckMode = true
		case "-quit-dirty":
			// Drives terminate: with a dirty document — the gesture -close-dirty
			// does NOT cover, because Quit never asks a window whether it should
			// close.
			quitCheckMode, quitDirty = true, true
		case "-menu":
			menuCheckMode = true
		case "-doc":
			docCheckMode = true
			if i+2 <= len(os.Args)-1 {
				docFixturePath = os.Args[i+2]
			}
		case "-close-dirty":
			closeCheckMode, closeDirty = true, true
		case "-modal":
			if i+2 <= len(os.Args)-1 {
				runModalCheckMode(parseStep(os.Args[i+2]))
				return true
			}
		}
	}
	return false
}
