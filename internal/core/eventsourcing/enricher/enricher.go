package enricher

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/store"
)

type Enricher struct {
	store store.EventStore
}

func NewEnricher(eventStore store.EventStore) *Enricher {
	return &Enricher{
		store: eventStore,
	}
}

func (e *Enricher) EnrichEvent(ctx context.Context, event domain.Event) error {
	if event.GetEventID() == "" {
		event.SetEventID(uuid.New().String())
	}

	if event.GetCorrelationID() == "" {
		event.SetCorrelationID(uuid.New().String())
	}

	if event.GetTimestamp().IsZero() {
		event.SetTimestamp(time.Now().UTC())
	}

	nextVersion, err := e.store.GetNextVersion(ctx, event.GetAggregateID())
	if err != nil {
		return err
	}

	event.SetAggregateVersion(nextVersion)

	return nil
}

