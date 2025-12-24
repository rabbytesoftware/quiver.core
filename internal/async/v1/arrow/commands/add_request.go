package commands

import (
	"context"
	"fmt"

	events "github.com/rabbytesoftware/quiver/internal/async/v1/arrow/events"
	errs "github.com/rabbytesoftware/quiver/internal/core/errs"
	"github.com/rabbytesoftware/quiver/internal/core/es/command"
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
	statics "github.com/rabbytesoftware/quiver/internal/models/statics"
)

type AddRequestInt interface {
	Validate(
		ctx context.Context,
		es command.EventSourcingValidator,
	) error

	ToEvent() (*event.Event[events.AddRequestedEvent], error)
}

type AddRequest struct {
	arrowNamespace string
	path           string
	forceAdd       bool
}

func NewAddRequest(
	arrowNamespace string,
	path string,
	forceAdd bool,
) AddRequestInt {
	return &AddRequest{
		arrowNamespace: arrowNamespace,
		path:           path,
		forceAdd:       forceAdd,
	}
}

func (r *AddRequest) Validate(
	ctx context.Context,
	es command.EventSourcingValidator,
) error {
	hasSucceeded, err := es.HasEventType(ctx, r.arrowNamespace, events.ArrowAddArrowSucceededType)
	if err != nil {
		return errs.InternalServerError(fmt.Sprintf("%s: %v", statics.FAILED_TO_CHECK_SUCCEEDED_EVENT, err))
	}
	if hasSucceeded {
		return errs.Conflict(fmt.Sprintf("%s: %s", statics.ARROW_ALREADY_EXISTS, r.arrowNamespace))
	}

	return nil
}

func (r *AddRequest) ToEvent() (*event.Event[events.AddRequestedEvent], error) {
	return events.NewAddRequestedEvent().
		WithArrowNamespace(r.arrowNamespace).
		WithPath(r.path).
		WithForceAdd(r.forceAdd).
		Build()
}
