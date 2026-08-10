package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	runtime2 "runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// wailsHost is the only implementation of hostPort today, and this file plus
// main.go are the only Go files that name Wails. Replacing the host means adding
// a sibling of this file and changing one line of main.
//
// Nothing above this boundary knows that assets reach the view through a custom
// URL scheme handled in-process, or that frontend calls arrive as a message
// string rather than a socket. Those are this file's business.
type wailsHost struct{}

func (wailsHost) Native() nativePort { return wailsNative{} }

func (wailsHost) Run(cfg hostConfig) error {
	return wails.Run(&options.App{
		Title:  cfg.Title,
		Width:  cfg.Width,
		Height: cfg.Height,
		AssetServer: &assetserver.Options{
			Assets: cfg.Assets,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		// The bundle advertises CFBundleDocumentTypes, so macOS routes a
		// double-clicked .md file here. At launch this fires before the webview
		// exists, which is why App holds the path until the frontend asks (#53).
		Mac: &mac.Options{
			OnFileOpen: cfg.OnFileOpen,
		},
		LogLevelProduction: logger.ERROR,
		OnStartup:          cfg.OnStartup,
		OnBeforeClose:      cfg.OnBeforeClose,
		Bind:               cfg.Bind,
	})
}

type wailsNative struct{}

func (wailsNative) OpenMarkdownFile(ctx context.Context) (string, error) {
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: "Open Markdown Document",
		Filters: []runtime.FileFilter{
			{DisplayName: "Markdown", Pattern: "*.md;*.markdown"},
			{DisplayName: "All Files", Pattern: "*"},
		},
	})
}

func (wailsNative) SaveMarkdownFile(ctx context.Context, defaultFilename string) (string, error) {
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:           "Save Markdown Document",
		DefaultFilename: defaultFilename,
		Filters:         []runtime.FileFilter{{DisplayName: "Markdown", Pattern: "*.md"}},
	})
}

func (wailsNative) SelectImageFile(ctx context.Context) (string, error) {
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: "Import Image",
		Filters: []runtime.FileFilter{
			{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg"},
			{DisplayName: "All Files", Pattern: "*"},
		},
	})
}

// SubscribeFileDrop forwards Wails file-drop paths to the frontend, which
// filters them to importable images.
func (wailsNative) SubscribeFileDrop(ctx context.Context, onDrop func(paths []string)) {
	runtime.OnFileDrop(ctx, func(_, _ int, paths []string) { onDrop(paths) })
}

func (wailsNative) EmitFilesDropped(ctx context.Context, paths []string) {
	runtime.EventsEmit(ctx, "files:dropped", paths)
}

// RevealPath shows the file in the platform's file browser rather than opening
// it, so the user lands on the asset inside its document asset folder.
//
// This ships as one binary for macOS, Windows and Linux, so the command is
// selected from GOOS rather than assumed. It previously always ran macOS's
// `open -R`, which fails silently everywhere else.
func (wailsNative) RevealPath(_ context.Context, path string) error {
	name, args := revealCommand(runtime2.GOOS, path)
	if name == "" {
		return fmt.Errorf("reveal: unsupported platform %s", runtime2.GOOS)
	}
	return exec.Command(name, args...).Run()
}

// revealCommand maps an OS to its reveal-in-file-browser invocation. Split out
// from RevealPath so the mapping is testable without executing anything.
func revealCommand(goos, path string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}
	case "windows":
		// /select, must be one argument with the path appended.
		return "explorer", []string{"/select," + path}
	case "linux":
		// No universal "select the file" verb; open the containing directory.
		return "xdg-open", []string{filepath.Dir(path)}
	default:
		return "", nil
	}
}

func (wailsNative) ConfirmOverwriteChanged(ctx context.Context, path string) (string, error) {
	return runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "File Changed on Disk",
		Message:       filepath.Base(path) + " has been modified by another program since you opened it.\n\nSaving now replaces those changes with your version.",
		Buttons:       []string{"Overwrite", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
		Icon:          iconFreeMessageDialogPNG,
	})
}

func (wailsNative) OpenExternalURL(ctx context.Context, url string) error {
	runtime.BrowserOpenURL(ctx, url)
	return nil
}

func (wailsNative) ShowError(ctx context.Context, title string, message string) {
	runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.ErrorDialog,
		Title:   title,
		Message: message,
		Icon:    iconFreeMessageDialogPNG,
	})
}

func (wailsNative) ConfirmUnsaved(ctx context.Context) (string, error) {
	return runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Unsaved Changes",
		Message:       "Save changes before closing?",
		Buttons:       []string{"Save", "Don't Save", "Cancel"},
		DefaultButton: "Save",
		CancelButton:  "Cancel",
		Icon:          iconFreeMessageDialogPNG,
	})
}

func (wailsNative) SetTitle(ctx context.Context, title string) {
	runtime.WindowSetTitle(ctx, title)
}

// EmitFileOpen tells the frontend to open a file macOS routed to us while the
// app was already running. The launch case is served by FrontendReady instead,
// because at launch there is no frontend to receive an event.
func (wailsNative) EmitFileOpen(ctx context.Context, path string) {
	runtime.EventsEmit(ctx, fileOpenEvent, path)
}
