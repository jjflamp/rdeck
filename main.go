package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"rdeck/app"
	"rdeck/internal/connections"
	"rdeck/internal/events"
	"rdeck/internal/redisclient"
	"rdeck/internal/settings"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	settingsDir := flag.String("settings-dir", defaultSettingsDir(), "application settings directory")
	flag.Parse()

	bus := events.New()
	store, err := connections.NewStore(*settingsDir)
	if err != nil {
		log.Fatalf("init connections store: %v", err)
	}
	settingsMgr, err := settings.NewManager(*settingsDir)
	if err != nil {
		log.Fatalf("init settings: %v", err)
	}
	connMgr := redisclient.NewManager(*settingsDir)

	bindings := app.New(bus, store, settingsMgr, connMgr)

	err = wails.Run(&options.App{
		Title:  "RDeck",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  bindings.Startup,
		OnShutdown: bindings.Shutdown,
		Bind: []interface{}{
			bindings,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Title:   "RDeck",
				Message: "Redis desktop client in Go",
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func defaultSettingsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	dir := filepath.Join(home, ".rdeck")
	// One-time migration from the pre-rename settings dir.
	legacy := filepath.Join(home, ".resp-go")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if _, err := os.Stat(legacy); err == nil {
			_ = os.Rename(legacy, dir)
		}
	}
	return dir
}
