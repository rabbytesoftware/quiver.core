package dto

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

type RunRecordDTO struct {
	Method    string            `json:"method" yaml:"method"`
	PID       int               `json:"pid,omitempty" yaml:"pid,omitempty"`
	Variables map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
	Steps     []StepProgressDTO `json:"steps,omitempty" yaml:"steps,omitempty"`
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
