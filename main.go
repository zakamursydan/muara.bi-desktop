package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:assets
var assets embed.FS

func main() {
	// The engine refuses to build a session in a CGO-free binary, so the app
	// exits here with the engine's own message instead of opening an empty
	// window — the negative proof M0 asks for.
	service, err := NewQueryService()
	if err != nil {
		log.Fatal(err)
	}
	defer service.Close()

	app := application.New(application.Options{
		Name:        "muara.bi spike",
		Description: "M0 platform spike",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "muara.bi · M0 spike",
		URL:    "/",
		Width:  1000,
		Height: 640,
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
