package eventsourcing

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type CommandBuilder struct {
	aggregateID      string
	eventFactory     func(version int64, metadata map[string]interface{}) domain.Event
	clientIP         string
	idempotencyKey   string
	requireExists    bool
	requireNotExists bool
	requireSucceeded string
}

func NewCommand(aggregateID string) *CommandBuilder {
	return &CommandBuilder{
		aggregateID: aggregateID,
	}
}

func (cb *CommandBuilder) WithEventFactory(factory func(version int64, metadata map[string]interface{}) domain.Event) *CommandBuilder {
	cb.eventFactory = factory
	return cb
}

func (cb *CommandBuilder) WithClientIP(clientIP string) *CommandBuilder {
	cb.clientIP = clientIP
	return cb
}

func (cb *CommandBuilder) WithIdempotencyKey(key string) *CommandBuilder {
	cb.idempotencyKey = key
	return cb
}

func (cb *CommandBuilder) RequireExists() *CommandBuilder {
	cb.requireExists = true
	return cb
}

func (cb *CommandBuilder) RequireNotExists() *CommandBuilder {
	cb.requireNotExists = true
	return cb
}

func (cb *CommandBuilder) RequireSucceeded(eventTypePrefix string) *CommandBuilder {
	cb.requireSucceeded = eventTypePrefix
	return cb
}

func (cb *CommandBuilder) Build() domain.Command {
	return &command{
		aggregateID:      cb.aggregateID,
		eventFactory:     cb.eventFactory,
		clientIP:         cb.clientIP,
		idempotencyKey:   cb.idempotencyKey,
		requireExists:    cb.requireExists,
		requireNotExists: cb.requireNotExists,
		requireSucceeded: cb.requireSucceeded,
	}
}

type command struct {
	aggregateID      string
	eventFactory     func(version int64, metadata map[string]interface{}) domain.Event
	clientIP         string
	idempotencyKey   string
	requireExists    bool
	requireNotExists bool
	requireSucceeded string
}

func (c *command) GetAggregateID() string {
	return c.aggregateID
}

func (c *command) Validate(ctx context.Context, es domain.EventSourcingValidator) error {
	if c.requireExists {
		exists, err := es.AggregateExists(ctx, c.aggregateID)
		if err != nil {
			return fmt.Errorf("failed to check aggregate existence: %w", err)
		}
		if !exists {
			return fmt.Errorf("aggregate %s does not exist", c.aggregateID)
		}
	}

	if c.requireNotExists {
		exists, err := es.AggregateExists(ctx, c.aggregateID)
		if err != nil {
			return fmt.Errorf("failed to check aggregate existence: %w", err)
		}
		if exists {
			return fmt.Errorf("aggregate %s already exists", c.aggregateID)
		}
	}

	if c.requireSucceeded != "" {
		hasSucceeded, err := es.HasSucceededEvent(ctx, c.aggregateID, c.requireSucceeded)
		if err != nil {
			return fmt.Errorf("failed to check succeeded event: %w", err)
		}
		if !hasSucceeded {
			return fmt.Errorf("required succeeded event %s not found for aggregate %s", c.requireSucceeded, c.aggregateID)
		}
	}

	return nil
}

func (c *command) ToRequestedEvent() domain.Event {
	metadata := make(map[string]interface{})
	if c.clientIP != "" {
		metadata["client_ip"] = c.clientIP
	}
	if c.idempotencyKey != "" {
		metadata["idempotency_key"] = c.idempotencyKey
	}

	if c.eventFactory == nil {
		return nil
	}

	return c.eventFactory(0, metadata)
}

