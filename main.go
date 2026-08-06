package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Dr. Markdown",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
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
