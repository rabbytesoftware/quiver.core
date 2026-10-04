package console

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
)

// ownSession always dials the daemon running the command, with the caller's
// bearer token, and has no CLI config to read.
type ownSession struct{ uri, token string }

func (ownSession) Config() (*config.Config, error) {
	return nil, errors.New("the CLI config is not available in the console")
}

func (s ownSession) Client(context.Context, *cobra.Command) (*client.Client, error) {
	if s.token == "" {
		return client.New(s.uri)
	}
	return client.New(s.uri, client.WithToken(s.token))
}

func (ownSession) Spinner(_ *cobra.Command, _ string, fn func() error) error { return fn() }

func (ownSession) IsTTY() bool { return false }
