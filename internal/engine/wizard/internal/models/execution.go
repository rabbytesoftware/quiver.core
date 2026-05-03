package models

import domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"

type Execution interface {
	Events() <-chan Event
	Done() <-chan struct{}
	Outcome() domainRuntime.ExecutionOutcome
}
