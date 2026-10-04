package console

import "github.com/rabbytesoftware/quiver.core/internal/console/command"

type commandsResponse struct {
	Commands []command.CommandInfo `json:"commands"`
}
