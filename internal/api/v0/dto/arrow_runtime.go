package dto

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

type ArrowRuntimeDTO struct {
	Namespace  string        `json:"namespace" yaml:"namespace"`
	State      string        `json:"state" yaml:"state"`
	ActiveRun  *RunRecordDTO `json:"active_run,omitempty" yaml:"active_run,omitempty"`
	LastReturn *ReturnDTO    `json:"last_return,omitempty" yaml:"last_return,omitempty"`
}

func ArrowRuntimeDTOFrom(rt domainRuntime.ArrowRuntime) ArrowRuntimeDTO {
	return ArrowRuntimeDTO{
		Namespace:  string(rt.Ref),
		State:      string(rt.State),
		ActiveRun:  RunRecordDTOFrom(rt.Execution),
		LastReturn: ReturnDTOFrom(rt.LastReturn),
	}
}
