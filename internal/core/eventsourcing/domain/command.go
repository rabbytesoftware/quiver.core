package domain

import "context"

type CommandInterface interface {
	Validate(ctx context.Context) error
	ToEvent() Event
}
