package models

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

type Execution interface {
	Events() <-chan Event
	Done() <-chan struct{}
	Outcome() domainRuntime.ExecutionOutcome
}
