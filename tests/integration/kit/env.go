package kit

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/stretchr/testify/require"
)

// Env is a fully wired test server.
type Env struct {
	URL       string
	closeOnce sync.Once
	closeFn   func()
}

// Close tears down the test server idempotently.
// It is safe to call multiple times (e.g., manually for ordered teardown and via t.Cleanup).
func (e *Env) Close() {
	e.closeOnce.Do(e.closeFn)
}

// Client returns a raw HTTP client pointed at this Env.
func (e *Env) Client(t *testing.T) *Client {
	return NewClient(t, e.URL)
}

// TypedClient returns a typed HTTP client pointed at this Env.
func (e *Env) TypedClient(t *testing.T) *TypedClient {
	return NewTypedClient(t, e.URL)
}

// BuildEnv wires a full test server using the given homeDir for path isolation.
// It registers e.Close via t.Cleanup so explicit Close calls are optional.
func BuildEnv(t *testing.T, repos FixtureRepos, home string) *Env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	rsv := newTestResolver(repos)
	engines.Manifold = manifold.NewWithResolvers(rsv, rsv)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	hub := api.NewHub()
	appContainer, err := app.New(engines, adapters, hub, app.WithHomeDir(home))
	require.NoError(t, err)

	apiContainer, err := api.New(appContainer, hub)
	require.NoError(t, err)

	srv := httptest.NewServer(apiContainer)
	closeFn := func() {
		srv.Close()
		cancel()
		// Give engine goroutines (asynx workers, process runners) time to drain
		// after context cancellation. Without this, goroutines accumulate and
		// saturate the CI runner's CPU, causing progressive slowdown.
		time.Sleep(500 * time.Millisecond)
	}
	e := &Env{URL: srv.URL, closeFn: closeFn}
	t.Cleanup(e.Close)
	return e
}

// NewEnv creates an Env with a fresh temp directory as its home.
func (s *IntegrationSuite) NewEnv() *Env {
	return BuildEnv(s.T(), s.Repos, s.T().TempDir())
}

// NewEnvWithHome creates an Env using an explicit home directory.
// Used for restart-survival tests that need two envs pointing at the same storage.
func (s *IntegrationSuite) NewEnvWithHome(home string) *Env {
	return BuildEnv(s.T(), s.Repos, home)
}
