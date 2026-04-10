package app

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
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

// Init constructs Arrow and Quiver services wired to the provided engine
// Container. Callers are responsible for opening and managing the event stores.
func Init(
	engines *engine.Container,
	arrowES asynxModels.Store,
	runtimeES asynxModels.Store,
	quiverES asynxModels.Store,
	hub apphub.WebSocketHub,
) (*Container, error) {
	os := domain.CurrentOS()

	arrowSvc, err := arrow.NewArrowBuilder().
		WithEngines(engines).
		WithEventStore(arrowES).
		WithRuntimeEventStore(runtimeES).
		WithOS(os).
		WithWebSocketHub(hub).
		Build()
	if err != nil {
		return nil, fmt.Errorf("app container: arrow: %w", err)
	}

	quiverSvc, err := quiver.NewQuiverBuilder().
		WithEngines(engines).
		WithEventStore(quiverES).
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
