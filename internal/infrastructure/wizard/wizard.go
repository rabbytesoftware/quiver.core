package wizard

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/wizard/events"
)

type Wizard struct {
	es *eventsourcing.EventSourcing
}

func NewWizard() *Wizard {
	es, err := eventsourcing.New().
		WithSQLiteStore(
			context.Background(),
			"wizard",
		).
		WithMemoryBus().
		Build()

	if err != nil {
		panic(err)
	}

	es.RegisterEvent(&events.ArrowAdded{})

	wizard := &Wizard{es: es}

	wizard.initLoggingProjection()

	return wizard
}

func (w *Wizard) Execute(
	ctx context.Context,
	cmd domain.CommandInterface,
) error {
	return w.es.Execute(ctx, cmd)
}

func (w *Wizard) initLoggingProjection() {
	w.es.Subscribe("*", func(ctx context.Context, event domain.Event) error {
		watcher.WithFields(map[string]interface{}{
			"event_type":     event.GetEventType(),
			"aggregate_type": event.GetAggregateType(),
			"aggregate_id":   event.GetAggregateID(),
			"timestamp":      event.GetTimestamp().Format("15:04:05"),
		}).Info("WizardEvent")
		return nil
	})
}
