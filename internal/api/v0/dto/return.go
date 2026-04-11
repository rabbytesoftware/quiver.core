package dto

import domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"

type ReturnDTO struct {
	Method    string            `json:"method"`
	Outcome   string            `json:"outcome"`
	Variables map[string]string `json:"variables,omitempty"`
}

func ReturnDTOFrom(r *domainRuntime.Return) *ReturnDTO {
	if r == nil {
		return nil
	}
	return &ReturnDTO{
		Method:    r.Method,
		Outcome:   string(r.Outcome),
		Variables: r.Variables,
	}
}
