package main

import (
	"context"
	"os"
	"os/signal"

	"puffin/internal/ui"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	app := ui.NewApp()
	status := app.Run(ctx)
	os.Exit(status)
}
