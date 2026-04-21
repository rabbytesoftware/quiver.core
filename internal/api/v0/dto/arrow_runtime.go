package dto

import domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"

type ArrowRuntimeDTO struct {
	Namespace  string        `json:"namespace"`
	State      string        `json:"state"`
	ActiveRun  *RunRecordDTO `json:"active_run,omitempty"`
	LastReturn *ReturnDTO    `json:"last_return,omitempty"`
}

func ArrowRuntimeDTOFrom(rt domainRuntime.ArrowRuntime) ArrowRuntimeDTO {
	return ArrowRuntimeDTO{
		Namespace:  string(rt.Ref),
		State:      string(rt.State),
		ActiveRun:  RunRecordDTOFrom(rt.Execution),
		LastReturn: ReturnDTOFrom(rt.LastReturn),
	}
}
