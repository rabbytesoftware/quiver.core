package dto

import domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"

type RunRecordDTO struct {
	Method    string            `json:"method"`
	Variables map[string]string `json:"variables,omitempty"`
}

func RunRecordDTOFrom(r *domainRuntime.RunRecord) *RunRecordDTO {
	if r == nil {
		return nil
	}
	return &RunRecordDTO{
		Method:    r.Method,
		Variables: r.Variables,
	}
}
