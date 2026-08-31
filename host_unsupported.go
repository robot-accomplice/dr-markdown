//go:build !darwin

package main

import (
	"context"
	"fmt"
)

// The application is macOS-only today: only tools/build-macos.sh exists, and no
// Windows or Linux artifact has ever been produced.
//
// This file exists so the package still COMPILES elsewhere, because CI runs the
// full suite on Linux. It refuses at run time rather than at build time, so a
// wrong platform is a clear message instead of a confusing link error.
func newHost() hostPort { return unsupportedHost{} }

type unsupportedHost struct{}

func (unsupportedHost) Native() nativePort { return unsupportedNative{} }

func (unsupportedHost) Run(hostConfig) error {
	return fmt.Errorf("Dr Markdown has no host for this platform yet; macOS only")
}

// unsupportedNative satisfies nativePort so the rest of the application type
// checks. Nothing reaches it: Run refuses before an app is ever started.
type unsupportedNative struct{}

func (unsupportedNative) OpenMarkdownFile(context.Context) (string, error)         { return "", nil }
func (unsupportedNative) SaveMarkdownFile(context.Context, string) (string, error) { return "", nil }
func (unsupportedNative) SelectImageFile(context.Context) (string, error)          { return "", nil }
func (unsupportedNative) RevealPath(context.Context, string) error                 { return nil }
func (unsupportedNative) OpenExternalURL(context.Context, string) error            { return nil }
func (unsupportedNative) SubscribeFileDrop(context.Context, func([]string))        {}
func (unsupportedNative) EmitFilesDropped(context.Context, []string)               {}
func (unsupportedNative) ShowError(context.Context, string, string)                {}
func (unsupportedNative) ConfirmUnsaved(context.Context) (string, error)           { return "", nil }
func (unsupportedNative) SetTitle(context.Context, string)                         {}
func (unsupportedNative) EmitFileOpen(context.Context, string)                     {}

func (unsupportedNative) ConfirmOverwriteChanged(context.Context, string) (string, error) {
	return "", nil
}

func (unsupportedNative) IsDefaultMarkdownHandler(context.Context) (bool, error) { return false, nil }
func (unsupportedNative) SetDefaultMarkdownHandler(context.Context) error        { return nil }

// runHarness is a no-op off macOS: the harness drives a native window.
func runHarness() bool { return false }
