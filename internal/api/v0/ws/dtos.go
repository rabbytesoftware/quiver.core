package ws

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// ArrowDTO is the WebSocket wire shape for domain.Arrow updates.
type ArrowDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Tags        []string `json:"tags"`
	Removed     bool     `json:"removed"`
}

// ArrowRuntimeDTO is the WebSocket wire shape for domainRuntime.ArrowRuntime updates.
type ArrowRuntimeDTO struct {
	Namespace  string        `json:"namespace"`
	State      string        `json:"state"`
	ActiveRun  *RunRecordDTO `json:"active_run,omitempty"`
	LastReturn *ReturnDTO    `json:"last_return,omitempty"`
}

// RunRecordDTO mirrors the active run inside ArrowRuntimeDTO.
type RunRecordDTO struct {
	Method    string            `json:"method"`
	Variables map[string]string `json:"variables,omitempty"`
}

// ReturnDTO mirrors the last return inside ArrowRuntimeDTO.
type ReturnDTO struct {
	Method  string `json:"method"`
	Outcome string `json:"outcome"`
}

// QuiverDTO is the WebSocket wire shape for domain.Quiver updates.
type QuiverDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Removed     bool     `json:"removed"`
}

func ArrowDTOFrom(a domain.Arrow) ArrowDTO {
	return ArrowDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Manifest.Name,
		Version:     a.Manifest.Version,
		Description: a.Manifest.Description,
		Tags:        a.Manifest.Tags,
		Removed:     a.Removed,
	}
}

func ArrowRuntimeDTOFrom(rt domainRuntime.ArrowRuntime) ArrowRuntimeDTO {
	dto := ArrowRuntimeDTO{
		Namespace: string(rt.Namespace),
		State:     string(rt.State),
	}
	if rt.ActiveRun != nil {
		dto.ActiveRun = &RunRecordDTO{
			Method:    rt.ActiveRun.Method,
			Variables: rt.ActiveRun.Variables,
		}
	}
	if rt.LastReturn != nil {
		dto.LastReturn = &ReturnDTO{
			Method:  rt.LastReturn.Method,
			Outcome: string(rt.LastReturn.Outcome),
		}
	}
	return dto
}

func QuiverDTOFrom(q domain.Quiver) QuiverDTO {
	return QuiverDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Manifest.Name,
		Description: q.Manifest.Description,
		Tags:        q.Manifest.Tags,
		Removed:     q.Removed,
	}
}
