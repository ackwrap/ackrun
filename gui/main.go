package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:loader
var loaderFiles embed.FS

func main() {
	assets, err := fs.Sub(loaderFiles, "loader")
	if err != nil {
		log.Fatalf("load GUI assets: %v", err)
	}
	app, err := NewApp()
	if err != nil {
		log.Fatalf("configure GUI: %v", err)
	}

	err = wails.Run(&options.App{
		Title:                            "Ackwrap",
		Width:                            1280,
		Height:                           820,
		MinWidth:                         960,
		MinHeight:                        640,
		HideWindowOnClose:                false,
		EnableDefaultContextMenu:         false,
		EnableFraudulentWebsiteDetection: false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnDomReady: app.domReady,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "595b3e64-c482-4ed6-9187-eb1f804db631",
			OnSecondInstanceLaunch: app.secondInstanceLaunch,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     false,
			DisableWebViewDrop: true,
		},
		BackgroundColour: &options.RGBA{R: 10, G: 17, B: 28, A: 255},
		Windows: &windows.Options{
			Theme:                windows.SystemDefault,
			DisablePinchZoom:     true,
			IsZoomControlEnabled: false,
			WindowClassName:      "AckwrapGUI",
		},
	})
	if err != nil {
		log.Fatalf("run GUI: %v", err)
	}
}
