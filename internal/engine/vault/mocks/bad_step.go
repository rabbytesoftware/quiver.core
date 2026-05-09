package mocks

import "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"

type BadStep struct {
	Unmarshalable chan struct{}
}

func (b *BadStep) Type() step.StepType {
	return step.StepTypeRun
}

func (b *BadStep) Title() string {
	return ""
}

func (b *BadStep) ExitOnFailure() bool {
	return false
}

func (b *BadStep) Resolve(_ string) step.Step {
	return b
}
