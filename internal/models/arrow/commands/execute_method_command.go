package commands

import (
	"context"
	"fmt"

	errs "github.com/rabbytesoftware/quiver/internal/core/errs"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	events "github.com/rabbytesoftware/quiver/internal/models/arrow/events"
	statics "github.com/rabbytesoftware/quiver/internal/models/statics"
)

type ExecuteMethodCommand interface {
	WithVariables(variables map[string]string) ExecuteMethodCommand
	WithClientIP(clientIP string) ExecuteMethodCommand
	GetAggregateID() string
	Validate(ctx context.Context, es domain.EventSourcingValidator) error
	ToRequestedEvent() domain.Event
}

type executeMethodCommand struct {
	namespace string
	method    string
	variables map[string]string
	clientIP  string
}

func NewExecuteMethodCommand(
	namespace, method string,
) ExecuteMethodCommand {
	return &executeMethodCommand{
		namespace: namespace,
		method:    method,
	}
}

func (c *executeMethodCommand) WithVariables(
	variables map[string]string,
) ExecuteMethodCommand {
	c.variables = variables
	return c
}

func (c *executeMethodCommand) WithClientIP(
	clientIP string,
) ExecuteMethodCommand {
	c.clientIP = clientIP
	return c
}

func (c *executeMethodCommand) GetAggregateID() string {
	return c.namespace
}

func (c *executeMethodCommand) Validate(
	ctx context.Context, 
	es domain.EventSourcingValidator,
) error {
	exists, err := es.AggregateExists(ctx, c.namespace)
	if err != nil {
		return errs.InternalServerError(fmt.Sprintf("%s: %v", statics.FAILED_TO_CHECK_AGGREGATE_EXISTENCE, err))
	}
	if !exists {
		return errs.NotFound(fmt.Sprintf("%s: %s", statics.AGGREGATE_DOES_NOT_EXIST, c.namespace))
	}

	hasSucceeded, err := es.HasEventType(ctx, c.namespace, events.ArrowAddArrowSucceededType)
	if err != nil {
		return errs.InternalServerError(fmt.Sprintf("%s: %v", statics.FAILED_TO_CHECK_SUCCEEDED_EVENT, err))
	}
	if !hasSucceeded {
		return errs.Conflict(fmt.Sprintf("%s: %s", statics.AGGREGATE_DOES_NOT_HAVE_SUCCEEDED_ADD_ARROW_EVENT, c.namespace))
	}

	hasStarted, err := es.HasEventType(ctx, c.namespace, events.ArrowExecuteMethodStartedType)
	if err != nil {
		return errs.InternalServerError(fmt.Sprintf("%s: %v", 
			statics.FAILED_TO_CHECK_STARTED_EVENT, 
			err,
		))
	}
	if hasStarted {
		return errs.Conflict(fmt.Sprintf("%s: %s", 
			statics.AGGREGATE_HAS_ACTIVE_EXECUTION, 
			c.namespace,
		))
	}

	return nil
}

func (c *executeMethodCommand) ToRequestedEvent() domain.Event {
	event := events.NewArrowExecuteMethodRequested(
		c.namespace, 
		c.method, 
		c.variables,
	)
	if c.clientIP != "" {
		metadata := event.GetMetadata()
		metadata["client_ip"] = c.clientIP
	}

	return event
}
