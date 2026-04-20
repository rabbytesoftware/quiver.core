//go:build integration

package integration_test

import (
	"context"
	"net/http/httptest"
	"time"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
)

type Env struct {
	URL   string
	home  string
	close func()
}

func (e *Env) Close() { e.close() }

func (s *IntegrationSuite) buildEnv(home string) *Env {
	ctx, cancel := context.WithCancel(context.Background())

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	s.Require().NoError(err)

	rsv := &testResolver{repos: s.repos}
	engines.Manifold = manifold.NewWithResolvers(rsv, rsv)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	s.Require().NoError(err)

	hub := api.NewHub()
	appContainer, err := app.New(engines, adapters, hub, app.WithHomeDir(home))
	s.Require().NoError(err)

	apiContainer, err := api.New(appContainer, hub)
	s.Require().NoError(err)

	srv := httptest.NewServer(apiContainer)
	closeAll := func() {
		srv.Close()
		cancel()
		// Give engine goroutines (asynx workers, process runners) time to drain
		// after context cancellation. Without this, goroutines from each test
		// accumulate and saturate the CI runner's CPU, causing progressive slowdown.
		time.Sleep(500 * time.Millisecond)
	}
	env := &Env{URL: srv.URL, home: home, close: closeAll}
	s.T().Cleanup(env.close)
	return env
}

func (s *IntegrationSuite) newEnv() *Env {
	return s.buildEnv(s.T().TempDir())
}

// newEnvWithHome creates an Env using an explicit home directory.
// Used for restart-survival tests that need two envs pointing at the same storage.
func (s *IntegrationSuite) newEnvWithHome(home string) *Env {
	return s.buildEnv(home)
}
