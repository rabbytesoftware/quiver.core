package commands

import (
	"context"
	"fmt"

	errs "github.com/rabbytesoftware/quiver/internal/core/errs"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	events "github.com/rabbytesoftware/quiver/internal/models/arrow/events"
	statics "github.com/rabbytesoftware/quiver/internal/models/statics"
)

type RemoveArrowCommand interface {
	WithForce(force bool) RemoveArrowCommand
	WithClientIP(clientIP string) RemoveArrowCommand
	GetAggregateID() string
	Validate(ctx context.Context, es domain.EventSourcingValidator) error
	ToRequestedEvent() domain.Event
}

type removeArrowCommand struct {
	namespace string
	force     bool
	clientIP  string
}

func NewRemoveArrowCommand(
	namespace string,
) RemoveArrowCommand {
	return &removeArrowCommand{
		namespace: namespace,
	}
}

func (c *removeArrowCommand) WithForce(
	force bool,
) RemoveArrowCommand {
	c.force = force
	return c
}

func (c *removeArrowCommand) WithClientIP(
	clientIP string,
) RemoveArrowCommand {
	c.clientIP = clientIP
	return c
}

func (c *removeArrowCommand) GetAggregateID() string {
	return c.namespace
}

func (c *removeArrowCommand) Validate(ctx context.Context, es domain.EventSourcingValidator) error {
	exists, err := es.AggregateExists(ctx, c.namespace)
	if err != nil {
		return errs.InternalServerError(fmt.Sprintf("%s: %v", statics.FAILED_TO_CHECK_AGGREGATE_EXISTENCE, err))
	}
	if !exists {
		return errs.NotFound(fmt.Sprintf("%s: %s", statics.AGGREGATE_DOES_NOT_EXIST, c.namespace))
	}
	return nil
}

func (c *removeArrowCommand) ToRequestedEvent() domain.Event {
	event := events.NewArrowRemoveArrowRequested(c.namespace, c.force)
	if c.clientIP != "" {
		metadata := event.GetMetadata()
		metadata["client_ip"] = c.clientIP
	}
	return event
}
