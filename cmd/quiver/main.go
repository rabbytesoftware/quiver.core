package main

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
)

func main() {
	internal := internal.NewInternal()

	go internal.Run()

	go watcher.Info(fmt.Sprintf(
		"%s %s '%s' - Initializing with embedded icon support...",
		metadata.GetName(),
		metadata.GetVersion(),
		metadata.GetVersionCodename(),
	))
}
