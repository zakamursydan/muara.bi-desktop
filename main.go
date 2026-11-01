package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The webview is served from the Vite build output, so `npm run build` in
// frontend/ has to run before `go build` — the same order Wails itself expects.
//
//go:embed all:frontend/dist
var bundles embed.FS

func bundledAssets() fs.FS {
	dist, err := fs.Sub(bundles, "frontend/dist")
	if err != nil {
		// Unreachable: the embed pattern guarantees the directory exists.
		log.Fatalf("frontend/dist is not embedded: %v", err)
	}
	return dist
}

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
			Handler: application.BundledAssetFileServer(bundledAssets()),
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
