package main

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
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
}
