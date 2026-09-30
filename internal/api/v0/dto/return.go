package dto

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

type ReturnDTO struct {
	// ExecutionID names the run this return ended; two returns of the same
	// method are the same run exactly when their IDs match.
	ExecutionID string            `json:"execution_id,omitempty" yaml:"execution_id,omitempty"`
	Method      string            `json:"method" yaml:"method"`
	Outcome     string            `json:"outcome" yaml:"outcome"`
	Variables   map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
	Steps       []StepProgressDTO `json:"steps,omitempty" yaml:"steps,omitempty"`
}

func ReturnDTOFrom(r *domainRuntime.Return) *ReturnDTO {
	if r == nil {
		return nil
	}
	steps := make([]StepProgressDTO, len(r.Steps))
	for i, sp := range r.Steps {
		steps[i] = StepProgressDTOFrom(sp)
	}
	return &ReturnDTO{
		ExecutionID: r.ExecutionID,
		Method:      r.Method,
		Outcome:     string(r.Outcome),
		Variables:   r.Variables,
		Steps:       steps,
	}
}
