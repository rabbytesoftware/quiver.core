package mocks

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// RuntimeCmd seeds an ArrowRuntime aggregate to a given state in tests.
type RuntimeCmd struct {
	NS    domain.Namespace
	State domain.ArrowState
}

func (c RuntimeCmd) AggregateID() string                          { return c.NS.String() }
func (c RuntimeCmd) EventName() string                            { return "runtime.mock" }
func (c RuntimeCmd) ShouldSnapshot() bool                         { return false }
func (c RuntimeCmd) Validate(_ *domainRuntime.ArrowRuntime) error { return nil }
func (c RuntimeCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{Namespace: c.NS, State: c.State}
}

// RuntimeCmdWithExecution seeds an ArrowRuntime with ActiveRun and LastReturn.
type RuntimeCmdWithExecution struct {
	NS         domain.Namespace
	State      domain.ArrowState
	ActiveRun  *domainRuntime.RunRecord
	LastReturn *domainRuntime.Return
}

func (c RuntimeCmdWithExecution) AggregateID() string                          { return c.NS.String() }
func (c RuntimeCmdWithExecution) EventName() string                            { return "runtime.mock.execution" }
func (c RuntimeCmdWithExecution) ShouldSnapshot() bool                         { return false }
func (c RuntimeCmdWithExecution) Validate(_ *domainRuntime.ArrowRuntime) error { return nil }
func (c RuntimeCmdWithExecution) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Namespace:  c.NS,
		State:      c.State,
		ActiveRun:  c.ActiveRun,
		LastReturn: c.LastReturn,
	}
}
