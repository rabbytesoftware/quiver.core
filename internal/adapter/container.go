package adapter

import (
	"fmt"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
)

type Container struct {
	ArrowES   asynxModels.Store
	RuntimeES asynxModels.Store
	QuiverES  asynxModels.Store
}

func Init(home string) (*Container, error) {
	arrowES, err := sqlite.NewEventStore(filepath.Join(home, "arrow-events.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: arrow event store: %w", err)
	}

	runtimeES, err := sqlite.NewEventStore(filepath.Join(home, "runtime-events.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: runtime event store: %w", err)
	}

	quiverES, err := sqlite.NewEventStore(filepath.Join(home, "quiver-events.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: quiver event store: %w", err)
	}

	return &Container{
		ArrowES:   arrowES,
		RuntimeES: runtimeES,
		QuiverES:  quiverES,
	}, nil
}
