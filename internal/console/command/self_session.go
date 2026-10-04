package command

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
)

var errConfigUnavailable = errors.New("the CLI configuration is not available in the console")

type selfSession struct {
	uri   string
	token string
}

func (s *selfSession) Config() (*config.Config, error) {
	return nil, errConfigUnavailable
}

func (s *selfSession) Client(
	_ context.Context,
	_ *cobra.Command,
) (*client.Client, error) {
	if s.token == "" {
		return client.New(s.uri)
	}
	return client.New(s.uri, client.WithToken(s.token))
}

func (s *selfSession) Spinner(
	_ *cobra.Command,
	_ string,
	fn func() error,
) error {
	return fn()
}

func (s *selfSession) IsTTY() bool {
	return false
}
