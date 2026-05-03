package mocks

import domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"

// Step is a configurable mock for domain/runtime/step.Step.
type Step struct {
	TypeVal          domainstep.StepType
	TitleVal         string
	ExitOnFailureVal bool
}

func (s Step) Type() domainstep.StepType        { return s.TypeVal }
func (s Step) Title() string                    { return s.TitleVal }
func (s Step) ExitOnFailure() bool              { return s.ExitOnFailureVal }
func (s Step) Resolve(_ string) domainstep.Step { return s }
