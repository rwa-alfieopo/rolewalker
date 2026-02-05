package main

import (
	"embed"
	_ "embed"
	"log"
	"os"
	"rolewalkers/cli"
	"rolewalkers/services"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// If no args or --gui flag, launch GUI
	// Otherwise run CLI
	if len(os.Args) == 1 || os.Args[1] == "--gui" || os.Args[1] == "gui" {
		runGUI()
	} else {
		cli.RunCLI()
	}
}

func runGUI() {
	// Initialize AWS service
	awsService, err := services.NewAWSService()
	if err != nil {
		log.Fatalf("Failed to initialize AWS service: %v", err)
	}

	// Create Wails application
	app := application.New(application.Options{
		Name:        "rolewalkers",
		Description: "AWS Profile & SSO Manager",
		Services: []application.Service{
			application.NewService(awsService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Create main window
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "rolewalkers - AWS Profile Manager",
		Width:  900,
		Height: 700,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})

	// Run the application
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
