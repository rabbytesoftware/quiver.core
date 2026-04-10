package arrows

import (
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// ExecuteMethodRequestDTO is the request body for POST /v1/arrow/:ns/:method.
type ExecuteMethodRequestDTO struct {
	Variables map[string]string `json:"variables"`
}

// ArrowListItemDTO is the wire shape for a single item in GET /v1/arrow.
type ArrowListItemDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Tags        []string `json:"tags"`
	Removed     bool     `json:"removed"`
}

// ArrowDetailDTO is the wire shape for GET /v1/arrow/:ns.
type ArrowDetailDTO struct {
	Namespace            string        `json:"namespace"`
	Name                 string        `json:"name"`
	Version              string        `json:"version"`
	Description          string        `json:"description"`
	License              string        `json:"license"`
	State                string        `json:"state"`
	Removed              bool          `json:"removed"`
	Tags                 []string      `json:"tags"`
	ActiveRun            *RunRecordDTO `json:"active_run,omitempty"`
	LastReturn           *ReturnDTO    `json:"last_return,omitempty"`
	IndirectDependencies []string      `json:"indirect_dependencies,omitempty"`
}

// RunRecordDTO is the wire shape for domainRuntime.RunRecord.
type RunRecordDTO struct {
	Method    string            `json:"method"`
	Variables map[string]string `json:"variables,omitempty"`
}

// ReturnDTO is the wire shape for domainRuntime.Return.
type ReturnDTO struct {
	Method    string            `json:"method"`
	Outcome   string            `json:"outcome"`
	Variables map[string]string `json:"variables,omitempty"`
}

func ToArrowListItemDTO(a arrow.ArrowListDTO) ArrowListItemDTO {
	return ArrowListItemDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Name,
		Version:     a.Version,
		Description: a.Description,
		State:       string(a.State),
		Tags:        a.Tags,
		Removed:     a.Removed,
	}
}

func ToArrowDetailDTO(a *arrow.ArrowDetailDTO) ArrowDetailDTO {
	dto := ArrowDetailDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Manifest.Name,
		Version:     a.Manifest.Version,
		Description: a.Manifest.Description,
		License:     a.Manifest.License,
		State:       string(a.State),
		Removed:     a.Removed,
		Tags:        a.Manifest.Tags,
	}
	if a.ActiveRun != nil {
		dto.ActiveRun = toRunRecordDTO(a.ActiveRun)
	}
	if a.LastReturn != nil {
		dto.LastReturn = toReturnDTO(a.LastReturn)
	}
	for _, dep := range a.IndirectDependencies {
		dto.IndirectDependencies = append(dto.IndirectDependencies, string(dep))
	}
	return dto
}

func toRunRecordDTO(r *domainRuntime.RunRecord) *RunRecordDTO {
	return &RunRecordDTO{
		Method:    r.Method,
		Variables: r.Variables,
	}
}

func toReturnDTO(r *domainRuntime.Return) *ReturnDTO {
	return &ReturnDTO{
		Method:    r.Method,
		Outcome:   string(r.Outcome),
		Variables: r.Variables,
	}
}
