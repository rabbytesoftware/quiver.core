// Package session resolves the daemon connection for one command
// invocation: loading the context store, booting a local unix:// daemon
// when needed, and — for tcp:// contexts with no cached token —
// transparently pairing a device and persisting the token it gets back.
package session

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

const spinnerDelay = 120 * time.Millisecond

// Deps injects the process-level collaborators session needs.
type Deps struct {
	// Version is the CLI build version.
	Version string
	// IsTTYFunc reports whether stdout is an interactive terminal.
	IsTTYFunc func() bool
	// EnsureDaemon boots the local daemon when the resolved server is a
	// Unix socket. nil disables daemon management (remote contexts, tests).
	EnsureDaemon func(ctx context.Context) error
}

// Flags is the subset of global flags session needs. The root command
// binds --server/--context/--config into this struct's fields.
type Flags struct {
	Server  string
	Context string
	Config  string
}

// Session resolves a connected client and the local config for one
// command invocation.
type Session interface {
	Config() (*config.Config, error)
	Client(ctx context.Context, cmd *cobra.Command) (*client.Client, error)
	Spinner(cmd *cobra.Command, label string, fn func() error) error
	IsTTY() bool
}

type session struct {
	deps  Deps
	flags *Flags
}

// New returns a Session reading deps and flags on every call, so flag
// values bound after cobra parsing are picked up.
func New(deps Deps, flags *Flags) Session {
	return &session{deps: deps, flags: flags}
}

func (s *session) IsTTY() bool {
	if s.deps.IsTTYFunc == nil {
		return false
	}
	return s.deps.IsTTYFunc()
}

// Config opens the context store at --config or the default path.
func (s *session) Config() (*config.Config, error) {
	path := s.flags.Config
	if path == "" {
		var err error
		path, err = config.DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	return config.Load(path)
}

func (s *session) Spinner(
	cmd *cobra.Command,
	label string,
	fn func() error,
) error {
	if !s.IsTTY() {
		return fn()
	}
	sp := tui.NewSpinner(cmd.ErrOrStderr(), label, spinnerDelay)
	sp.Start()
	defer sp.Stop()
	return fn()
}

// Client resolves the target server and returns a connected client,
// booting the local daemon first when applicable and, for a tcp:// context
// with no cached token, wiring in automatic pairing on the first 401.
func (s *session) Client(
	ctx context.Context,
	cmd *cobra.Command,
) (*client.Client, error) {
	cfg, err := s.Config()
	if err != nil {
		return nil, err
	}

	server, err := cfg.Resolve(s.flags.Server, s.flags.Context)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(server, "unix://") {
		return s.unixClient(cmd, server)
	}

	return s.tcpClient(cfg, server)
}

func (s *session) unixClient(
	cmd *cobra.Command,
	server string,
) (*client.Client, error) {
	if s.deps.EnsureDaemon != nil {
		if err := s.Spinner(cmd, "starting daemon", func() error {
			return s.deps.EnsureDaemon(cmd.Context())
		}); err != nil {
			return nil, err
		}
	}
	return client.New(server)
}

func (s *session) tcpClient(
	cfg *config.Config,
	server string,
) (*client.Client, error) {
	ctxName, token := s.activeContext(cfg)

	opts := []client.Option{
		client.WithUnauthorizedHandler(s.pairAndPersist(cfg, ctxName)),
	}
	if token != "" {
		opts = append(opts, client.WithToken(token))
	}

	return client.New(server, opts...)
}

// activeContext returns the resolved context's name and stored token. An
// explicit --server override has no named context to read a token from or
// persist one into — it resolves as ("", "") and re-pairs every invocation.
func (s *session) activeContext(cfg *config.Config) (name, token string) {
	if s.flags.Server != "" {
		return "", ""
	}

	name = s.flags.Context
	if name == "" {
		name = cfg.ActiveName()
	}

	ctx, err := cfg.Get(name)
	if err != nil {
		return "", ""
	}
	return ctx.Name, ctx.Token
}

// pairAndPersist builds the client.UnauthorizedHandler for a tcp://
// session: generate a pairing code and redeem it immediately — both calls
// are loopback-gated with no token of their own, which is what makes this
// automatic (see the design spec's rationale on why the CLI can do this
// without the human-in-the-loop step quiver.desktop needs). ctxName empty
// means the caller used an ephemeral --server override, so the token is
// used for this invocation only and never written to disk.
func (s *session) pairAndPersist(
	cfg *config.Config,
	ctxName string,
) client.UnauthorizedHandler {
	return func(ctx context.Context, c *client.Client) (string, error) {
		pc, err := c.GeneratePairingCode(ctx)
		if err != nil {
			return "", fmt.Errorf("session: generate pairing code: %w", err)
		}

		deviceID, label := deviceIdentity()

		token, err := c.RedeemPairingCode(ctx, pc.Code, deviceID, label)
		if err != nil {
			return "", fmt.Errorf("session: redeem pairing code: %w", err)
		}

		if ctxName != "" {
			if err := cfg.SetToken(ctxName, token); err != nil {
				return "", fmt.Errorf("session: persist device token: %w", err)
			}
		}

		return token, nil
	}
}

func deviceIdentity() (id, label string) {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return "cli-" + host, "cli@" + host
}
