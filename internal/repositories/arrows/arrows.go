package arrows

import (
	"context"
	"fmt"

	errors "github.com/rabbytesoftware/quiver/internal/core/errs"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	domain "github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/arrow/commands"
	events "github.com/rabbytesoftware/quiver/internal/models/arrow/events"
	statics "github.com/rabbytesoftware/quiver/internal/models/statics"
)

type ArrowsRepository struct {
	es *eventsourcing.EventSourcing
}

func NewArrowsRepository() ArrowsInterface {
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
	es.RegisterEvent(&events.ArrowAddArrowSucceeded{})
	es.RegisterEvent(&events.ArrowAddArrowFailed{})
	es.RegisterEvent(&events.ArrowRemoveArrowRequested{})
	es.RegisterEvent(&events.ArrowRemoveArrowSucceeded{})
	es.RegisterEvent(&events.ArrowRemoveArrowFailed{})
	es.RegisterEvent(&events.ArrowExecuteMethodRequested{})
	es.RegisterEvent(&events.ArrowExecuteMethodStarted{})
	es.RegisterEvent(&events.ArrowExecuteMethodProgressUpdated{})
	es.RegisterEvent(&events.ArrowExecuteMethodSucceeded{})
	es.RegisterEvent(&events.ArrowExecuteMethodFailed{})

	return &ArrowsRepository{
		es: es,
	}
}

func (r *ArrowsRepository) AddArrow(
	ctx context.Context,
	namespace, path string,
	force bool,
	clientIP string,
) (
	arrow domain.Arrow,
	warnings []error,
	err error,
) {
	cmd := commands.NewAddArrowCommand(namespace, path).
		WithForce(force).
		WithClientIP(clientIP)

	if err := r.es.ExecuteCommand(ctx, cmd); err != nil {
		return arrow, nil, err
	}

	return arrow, warnings, nil
}

func (r *ArrowsRepository) DeleteArrow(
	ctx context.Context,
	namespace string,
	force bool,
	clientIP string,
) (
	warnings []error,
	err error,
) {
	cmd := commands.NewRemoveArrowCommand(namespace).
		WithForce(force).
		WithClientIP(clientIP)

	if err := r.es.ExecuteCommand(ctx, cmd); err != nil {
		return nil, err
	}

	return warnings, nil
}

func (r *ArrowsRepository) ExecuteMethod(
	ctx context.Context,
	namespace, method string,
	variables map[string]string,
	clientIP string,
) (
	warnings []error,
	err error,
) {
	cmd := commands.NewExecuteMethodCommand(namespace, method).
		WithVariables(variables).
		WithClientIP(clientIP)

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
		return arrow, nil, errors.InternalServerError(fmt.Sprintf("%s: %s", 
			statics.FAILED_TO_CHECK_AGGREGATE_EXISTENCE,
			namespace,
		))
	}

	if !exists {
		return arrow, nil, errors.NotFound(fmt.Sprintf("%s: %s", 
			statics.ARROW_NOT_FOUND,
			namespace,
		))
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
