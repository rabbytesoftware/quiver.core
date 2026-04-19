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

// New constructs Arrow and Quiver services wired to the provided engine
// Container. Callers are responsible for opening and managing the event stores.
func New(
	engines *engine.Container,
	adapters *adapter.Container,
	hub apphub.WebSocketHub,
) (*Container, error) {
	os := domain.CurrentOS()

	arrowSvc, err := arrow.NewArrowBuilder().
		WithEngines(engines).
		WithEventStore(adapters.ArrowES).
		WithRuntimeEventStore(adapters.RuntimeES).
		WithOS(os).
		WithWebSocketHub(hub).
		Build()
	if err != nil {
		return nil, fmt.Errorf("app container: arrow: %w", err)
	}

	quiverSvc, err := quiver.NewQuiverBuilder().
		WithEngines(engines).
		WithEventStore(adapters.QuiverES).
		WithWebSocketHub(hub).
		Build()
	if err != nil {
		return nil, fmt.Errorf("app container: quiver: %w", err)
	}

	return &Container{
		Arrow:  arrowSvc,
		Quiver: quiverSvc,
	}, nil
}
