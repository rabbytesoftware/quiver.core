package app

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/app/quiver"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

// Container holds all app-layer services.
type Container struct {
	Arrow  arrow.ArrowService
	Quiver quiver.QuiverService
}

type appOpts struct{ homeDir string }

// Option configures app.New.
type Option func(*appOpts)

// WithHomeDir overrides the home directory used for path resolution.
func WithHomeDir(dir string) Option {
	return func(o *appOpts) { o.homeDir = dir }
}

// New constructs Arrow and Quiver services wired to the provided engine
// Container. Callers are responsible for opening and managing the event stores.
func New(
	engines *engine.Container,
	adapters *adapter.Container,
	hub apphub.WebSocketHub,
	opts ...Option,
) (*Container, error) {
	cfg := appOpts{}
	for _, o := range opts {
		o(&cfg)
	}
	os := domain.CurrentOS()

	arrowBuilder := arrow.NewArrowBuilder().
		WithEngines(engines).
		WithEventStore(adapters.ArrowES).
		WithRuntimeEventStore(adapters.RuntimeES).
		WithOS(os).
		WithWebSocketHub(hub)
	if cfg.homeDir != "" {
		arrowBuilder = arrowBuilder.WithHomeDir(cfg.homeDir)
	}
	arrowSvc, err := arrowBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("app container: arrow: %w", err)
	}

	quiverBuilder := quiver.NewQuiverBuilder().
		WithEngines(engines).
		WithEventStore(adapters.QuiverES).
		WithWebSocketHub(hub)
	if cfg.homeDir != "" {
		quiverBuilder = quiverBuilder.WithHomeDir(cfg.homeDir)
	}
	quiverSvc, err := quiverBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("app container: quiver: %w", err)
	}

	return &Container{
		Arrow:  arrowSvc,
		Quiver: quiverSvc,
	}, nil
}
