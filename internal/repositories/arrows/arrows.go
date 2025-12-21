package arrows

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	domain "github.com/rabbytesoftware/quiver/internal/models/arrow"
	events "github.com/rabbytesoftware/quiver/internal/models/events/arrows"
)

type ArrowsRepository struct {
	es *eventsourcing.EventSourcing
}

func NewArrowsRepository(
	_ *infrastructure.Infrastructure,
) ArrowsInterface {
	es, err := eventsourcing.New().
		WithSQLiteStore(
			context.Background(),
			"arrows",
		).
		WithMemoryBus().
		Build()

	if err != nil {
		panic(fmt.Errorf("failed to initialize arrows repository event sourcing: %w", err))
	}

	es.RegisterEvent(&events.ArrowAddArrowRequested{})
	es.RegisterEvent(&events.ArrowRemoveArrowRequested{})
	es.RegisterEvent(&events.ArrowExecuteMethodRequested{})

	return &ArrowsRepository{
		es: es,
	}
}

func (r *ArrowsRepository) AddArrow(
	ctx context.Context,
	namespace, path string,
	force bool,
	idempotencyKey, clientIP string,
) (
	arrow domain.Arrow,
	warnings []error,
	err error,
) {
	cmd := eventsourcing.NewCommand(namespace).
		WithEventFactory(func(version int64, metadata map[string]interface{}) events.Event {
			return events.NewArrowAddArrowRequested(
				namespace,
				path,
				force,
				version,
				"",
				nil,
				metadata,
			)
		}).
		WithClientIP(clientIP).
		WithIdempotencyKey(idempotencyKey).
		RequireNotExists().
		RequireSucceeded("arrow.AddArrow").
		Build()

	if err := r.es.ExecuteCommand(ctx, cmd); err != nil {
		return arrow, nil, err
	}

	return arrow, warnings, nil
}

func (r *ArrowsRepository) DeleteArrow(
	ctx context.Context,
	namespace string,
	force bool,
	idempotencyKey, clientIP string,
) (
	warnings []error,
	err error,
) {
	cmd := eventsourcing.NewCommand(namespace).
		WithEventFactory(func(version int64, metadata map[string]interface{}) events.Event {
			return events.NewArrowRemoveArrowRequested(
				namespace,
				force,
				version,
				"",
				nil,
				metadata,
			)
		}).
		WithClientIP(clientIP).
		WithIdempotencyKey(idempotencyKey).
		RequireExists().
		Build()

	if err := r.es.ExecuteCommand(ctx, cmd); err != nil {
		return nil, err
	}

	return warnings, nil
}

func (r *ArrowsRepository) ExecuteMethod(
	ctx context.Context,
	namespace, method string,
	variables map[string]string,
	idempotencyKey, clientIP string,
) (
	warnings []error,
	err error,
) {
	cmd := eventsourcing.NewCommand(namespace).
		WithEventFactory(func(version int64, metadata map[string]interface{}) events.Event {
			return events.NewArrowExecuteMethodRequested(
				namespace,
				method,
				variables,
				version,
				"",
				nil,
				metadata,
			)
		}).
		WithClientIP(clientIP).
		WithIdempotencyKey(idempotencyKey).
		RequireExists().
		Build()

	if err := r.es.ExecuteCommand(ctx, cmd); err != nil {
		return nil, err
	}

	return warnings, nil
}

func (r *ArrowsRepository) StopMethod(
	ctx context.Context,
	namespace, method string,
) (
	warnings []error,
	err error,
) {
	// TODO: Implement StopMethod
	return nil, fmt.Errorf("not implemented")
}

func (r *ArrowsRepository) GetArrow(
	ctx context.Context,
	namespace string,
) (
	arrow domain.Arrow,
	warnings []error,
	err error,
) {
	exists, err := r.es.AggregateExists(ctx, namespace)
	if err != nil {
		return arrow, nil, fmt.Errorf("failed to check aggregate existence: %w", err)
	}

	if !exists {
		return arrow, nil, fmt.Errorf("arrow not found: %s", namespace)
	}

	return arrow, warnings, nil
}

func (r *ArrowsRepository) ListArrows(
	ctx context.Context,
) (
	arrows map[string]ArrowState,
	warnings []error,
	err error,
) {
	// TODO: Query from projection
	// For now, return empty map (projection will be implemented later)
	return make(map[string]ArrowState), warnings, nil
}
