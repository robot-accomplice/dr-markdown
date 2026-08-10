package main

import "embed"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	host := wailsHost{}
	app := NewApp(host.Native())

	err := host.Run(hostConfig{
		Title:         "Dr. Markdown",
		Width:         1440,
		Height:        900,
		Assets:        assets,
		OnStartup:     app.startup,
		OnBeforeClose: app.beforeClose,
		OnFileOpen:    app.openFileFromOS,
		Bind:          []interface{}{app},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
