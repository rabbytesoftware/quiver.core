package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/app/quiver"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

// Container holds all app-layer services.
type Container struct {
	Arrow  arrow.ArrowService
	Quiver quiver.QuiverService
}

// Init constructs Arrow and Quiver services wired to the provided engine
// Container. Each service gets its own SQLite event store under ~/.quiver/events/.
func Init(
	engines *engine.Container,
) (*Container, error) {
	arrowES, err := openEventStore("arrows.db")
	if err != nil {
		return nil, fmt.Errorf("app container: %w", err)
	}

	runtimeES, err := openEventStore("runtime.db")
	if err != nil {
		return nil, fmt.Errorf("app container: %w", err)
	}

	quiverES, err := openEventStore("quivers.db")
	if err != nil {
		return nil, fmt.Errorf("app container: %w", err)
	}

	os := domain.CurrentOS()

	arrowSvc, err := arrow.NewArrowBuilder().
		WithEngines(engines).
		WithEventStore(arrowES).
		WithRuntimeEventStore(runtimeES).
		WithOS(os).
		Build()
	if err != nil {
		return nil, fmt.Errorf("app container: arrow: %w", err)
	}

	quiverSvc, err := quiver.NewQuiverBuilder().
		WithEngines(engines).
		WithEventStore(quiverES).
		Build()
	if err != nil {
		return nil, fmt.Errorf("app container: quiver: %w", err)
	}

	return &Container{
		Arrow:  arrowSvc,
		Quiver: quiverSvc,
	}, nil
}

func openEventStore(
	filename string,
) (models.Store, error) {
	dir := filepath.Join(metadata.GetQuiverHome(), "events")

	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create events dir: %w", err)
	}

	return sqlite.NewEventStore(
		filepath.Join(dir, filename),
	)
}
