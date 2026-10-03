package dto

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

type ArrowRuntimeDTO struct {
	Namespace  string        `json:"namespace" yaml:"namespace"`
	State      string        `json:"state" yaml:"state"`
	ActiveRun  *RunRecordDTO `json:"active_run,omitempty" yaml:"active_run,omitempty"`
	LastReturn *ReturnDTO    `json:"last_return,omitempty" yaml:"last_return,omitempty"`
	// PendingActivation is a staged binary waiting for a daemon restart, null
	// when nothing is staged.
	PendingActivation *PendingActivationDTO `json:"pending_activation" yaml:"pending_activation"`
	// Settling is true while an update has not committed yet, including the
	// moment after its steps ended and before its row advanced. Only REST
	// reads set it; streamed runtime events omit it.
	Settling bool `json:"settling,omitempty" yaml:"settling,omitempty"`
}

func ArrowRuntimeDTOFrom(rt domainRuntime.ArrowRuntime) ArrowRuntimeDTO {
	return ArrowRuntimeDTO{
		Namespace:         string(rt.Ref),
		State:             string(rt.State),
		ActiveRun:         RunRecordDTOFrom(rt.Execution),
		LastReturn:        ReturnDTOFrom(rt.LastReturn),
		PendingActivation: PendingActivationDTOFrom(rt.PendingActivation),
	}
}
