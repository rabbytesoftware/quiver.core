package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal"
	v1 "github.com/rabbytesoftware/quiver/internal/api/v1"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
	"github.com/rabbytesoftware/quiver/internal/ws"
)

func main() {
	internal := internal.NewInternal()

	watcher.Info(fmt.Sprintf(
		"%s %s '%s' - Starting Quiver...",
		metadata.GetName(),
		metadata.GetVersion(),
		metadata.GetVersionCodename(),
	))

	internal.Run()

	r := gin.Default()

	handler := ws.NewMockWebSocketHandler()
	api := r.Group("/api/v1")
	v1.RegisterWebSocketRoutes(api, handler)
	r.Run(":8000")
}
