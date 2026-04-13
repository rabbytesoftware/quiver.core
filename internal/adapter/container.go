package adapter

import (
	"fmt"
	"os"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
)

type Container struct {
	ArrowES   asynxModels.Store
	RuntimeES asynxModels.Store
	QuiverES  asynxModels.Store
}

func Init() (*Container, error) {
	eventsPath := metadata.GetEventsPath()
	if err := os.MkdirAll(eventsPath, 0750); err != nil {
		return nil, fmt.Errorf("adapter: create events dir: %w", err)
	}

	arrowES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "arrow.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: arrow event store: %w", err)
	}

	runtimeES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "runtime.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: runtime event store: %w", err)
	}

	quiverES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "quiver.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: quiver event store: %w", err)
	}

	return &Container{
		ArrowES:   arrowES,
		RuntimeES: runtimeES,
		QuiverES:  quiverES,
	}, nil
}
