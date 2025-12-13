package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

	"fyne.io/fyne/v2/app"
	grottoApp "github.com/shhac/grotto/internal/app"
	"github.com/shhac/grotto/internal/ui"
)

func main() {
	if err := runApp(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

// runApp is the main application entry point with panic recovery.
func runApp() (err error) {
	// Create a temporary stdout logger for bootstrap errors
	tempLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			tempLogger.Error("panic recovered",
				slog.Any("panic", r),
				slog.String("stack", string(debug.Stack())),
			)
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	tempLogger.Info("starting Grotto gRPC client")

	// Load configuration from environment
	cfg := grottoApp.ConfigFromEnv()

	// Create Fyne application
	fyneApp := app.NewWithID("com.grotto.client")

	// Create and wire the application
	grottoApp, err := grottoApp.New(fyneApp, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}

	// Create main window
	mainWindow := ui.NewMainWindow(
		grottoApp.FyneApp(),
		grottoApp, // Pass the app as the controller
	)

	// Run the application (blocking)
	grottoApp.Run(mainWindow.Window())

	grottoApp.Logger().Info("application shutdown complete")
	return nil
}
