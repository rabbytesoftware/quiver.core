package dto

import domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"

type ReturnDTO struct {
	Method    string            `json:"method"`
	Outcome   string            `json:"outcome"`
	Variables map[string]string `json:"variables,omitempty"`
	Steps     []StepProgressDTO `json:"steps,omitempty"`
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
		Method:    r.Method,
		Outcome:   string(r.Outcome),
		Variables: r.Variables,
		Steps:     steps,
	}
}
