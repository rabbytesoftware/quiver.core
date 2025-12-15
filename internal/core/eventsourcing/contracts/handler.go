package contracts

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type Handler func(context.Context, domain.Event) error
