package console

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
)

type ownSession struct {
	uri   string
	token string
}

func newOwnSession(
	c *gin.Context,
) ownSession {
	addr, _ := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr)
	token, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	return ownSession{uri: gateway.DialURI(addr), token: strings.TrimSpace(token)}
}

// Config always fails: a console command never reads the CLI's config file.
func (s ownSession) Config() (*config.Config, error) {
	return nil, errors.New("the CLI config is not available in the console")
}

// Client returns a client for the daemon that is running the command, carrying
// the caller's own bearer token when it sent one. The --server, --context and
// --config flags have no say in where it connects.
func (s ownSession) Client(
	_ context.Context,
	_ *cobra.Command,
) (*client.Client, error) {
	if s.token == "" {
		return client.New(s.uri)
	}
	return client.New(s.uri, client.WithToken(s.token))
}

// Spinner runs fn without drawing anything: console output is plain text.
func (s ownSession) Spinner(
	_ *cobra.Command,
	_ string,
	fn func() error,
) error {
	return fn()
}

// IsTTY is always false, so commands render plain text and never prompt.
func (s ownSession) IsTTY() bool {
	return false
}
