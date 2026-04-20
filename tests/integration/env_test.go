//go:build integration

package integration_test

import (
	"context"
	"net/http/httptest"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
)

type Env struct {
	URL  string
	home string
}

func (s *IntegrationSuite) newEnv() *Env {
	home := s.T().TempDir()
	s.T().Setenv("HOME", home)

	ctx := context.Background()

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
	s.T().Cleanup(srv.Close)

	return &Env{URL: srv.URL, home: home}
}

// newEnvWithHome creates an Env using an explicit home directory.
// Used for restart-survival tests that need two envs pointing at the same storage.
func (s *IntegrationSuite) newEnvWithHome(home string) *Env {
	s.T().Setenv("HOME", home)

	ctx := context.Background()

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
	s.T().Cleanup(srv.Close)

	return &Env{URL: srv.URL, home: home}
}
