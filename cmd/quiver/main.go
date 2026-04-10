package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/rabbytesoftware/quiver/internal"
)

func main() {
	ctx := context.Background()

	container, err := internal.Init(ctx)
	if err != nil {
		slog.Error("failed to initialize", "error", err)
		os.Exit(1)
	}

	addr := ":8080"
	slog.Info("starting quiver server", "addr", addr)
	if err := container.API.Run(addr); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
