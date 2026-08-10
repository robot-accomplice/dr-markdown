package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Dr. Markdown",
		Width:  1440,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		// The bundle advertises CFBundleDocumentTypes, so macOS routes a
		// double-clicked .md file here. Nothing consumed it until now: the app
		// opened an empty document and the file the user clicked was silently
		// gone (#53). At launch this fires before the webview exists, which is
		// why App holds the path until the frontend asks for it.
		Mac: &mac.Options{
			OnFileOpen: app.openFileFromOS,
		},
		LogLevelProduction: logger.ERROR,
		OnStartup:          app.startup,
		OnBeforeClose:      app.beforeClose,
		Bind:               []interface{}{app},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
