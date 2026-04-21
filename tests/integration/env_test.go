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

func (e *Env) Close() {
	e.close()
}

func (s *IntegrationSuite) buildEnv(home string) *Env {
	// HOME must be set before engine.New so all path resolution uses temp dir
	s.T().Setenv("HOME", home)

	ctx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- cancel called in closeFn

	engines, err := engine.New(ctx)
	s.Require().NoError(err)

	rsv := &testResolver{repos: s.repos}
	engines.Manifold = manifold.NewWithResolvers(rsv, rsv)

	adapters, err := adapter.New()
	s.Require().NoError(err)

	hub := api.NewHub()
	appContainer, err := app.New(engines, adapters, hub)
	s.Require().NoError(err)

	apiContainer, err := api.New(appContainer, hub)
	s.Require().NoError(err)

	srv := httptest.NewServer(apiContainer)
	closeFn := func() {
		// srv.Close() must run before cancel() so active HTTP handlers finish first.
		srv.Close()
		cancel()
		// Give engine goroutines (asynx workers, SQLite connections) time to drain
		// after context cancellation. Without this the temp dir cleanup races with
		// goroutines still holding the SQLite file open.
		time.Sleep(2 * time.Second)
	}
	env := &Env{URL: srv.URL, home: home, close: closeFn}
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
