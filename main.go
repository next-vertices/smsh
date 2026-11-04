package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"smsh/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a, err := app.New()
	if err != nil {
		log.Fatal(err)
	}

	err = wails.Run(&options.App{
		Title:     "Smsh",
		Width:     1100,
		Height:    760,
		MinWidth:  480,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1},
		OnStartup:        a.Startup,
		Bind:             []interface{}{a},
	})
	if err != nil {
		log.Fatal(err)
	}
}
