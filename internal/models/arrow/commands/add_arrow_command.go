package commands

import (
	"context"
	"fmt"

	errs "github.com/rabbytesoftware/quiver/internal/core/errs"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	events "github.com/rabbytesoftware/quiver/internal/models/arrow/events"
	statics "github.com/rabbytesoftware/quiver/internal/models/statics"
)

type AddArrowCommand interface {
	WithForce(force bool) AddArrowCommand
	WithClientIP(clientIP string) AddArrowCommand
	GetAggregateID() string
	Validate(ctx context.Context, es domain.EventSourcingValidator) error
	ToRequestedEvent() domain.Event
}

type addArrowCommand struct {
	namespace string
	path      string
	force     bool
	clientIP  string
}

func NewAddArrowCommand(
	namespace, path string,
) AddArrowCommand {
	return &addArrowCommand{
		namespace: namespace,
		path:      path,
	}
}

func (c *addArrowCommand) WithForce(
	force bool,
) AddArrowCommand {
	c.force = force
	return c
}

func (c *addArrowCommand) WithClientIP(
	clientIP string,
) AddArrowCommand {
	c.clientIP = clientIP
	return c
}

func (c *addArrowCommand) GetAggregateID() string {
	return c.namespace
}

func (c *addArrowCommand) Validate(
	ctx context.Context,
	es domain.EventSourcingValidator,
) error {
	exists, err := es.AggregateExists(ctx, c.namespace)
	if err != nil {
		return errs.InternalServerError(fmt.Sprintf("%s: %v", statics.FAILED_TO_CHECK_AGGREGATE_EXISTENCE, err))
	}

	if !exists && c.force {
		return nil // ? If the aggregate does not exist and force is true, we can add it
	}

	hasSucceeded, err := es.HasEventType(ctx, c.namespace, events.ArrowAddArrowSucceededType)
	if err != nil {
		return errs.InternalServerError(fmt.Sprintf("%s: %v", statics.FAILED_TO_CHECK_SUCCEEDED_EVENT, err))
	}
	
	if hasSucceeded {
		return errs.Conflict(fmt.Sprintf("%s: %s", statics.ARROW_ALREADY_EXISTS, c.namespace))
	}

	return nil
}

func (c *addArrowCommand) ToRequestedEvent() domain.Event {
	event := events.NewArrowAddArrowRequested(
		c.namespace,
		c.path,
		c.force,
	)
	if c.clientIP != "" {
		metadata := event.GetMetadata()
		metadata["client_ip"] = c.clientIP
	}
	return event
}
