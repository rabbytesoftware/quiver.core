package dto

import domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"

type RunRecordDTO struct {
	Method    string            `json:"method"`
	PID       int               `json:"pid,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	Steps     []StepProgressDTO `json:"steps,omitempty"`
}

func RunRecordDTOFrom(r *domainRuntime.Execution) *RunRecordDTO {
	if r == nil {
		return nil
	}
	steps := make([]StepProgressDTO, len(r.Steps))
	for i, sp := range r.Steps {
		steps[i] = StepProgressDTOFrom(sp)
	}
	return &RunRecordDTO{
		Method:    r.Method,
		PID:       r.PID,
		Variables: r.Variables,
		Steps:     steps,
	}
}
