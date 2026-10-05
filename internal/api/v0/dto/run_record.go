package dto

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

type RunRecordDTO struct {
	Method    string            `json:"method" yaml:"method"`
	PID       int               `json:"pid,omitempty" yaml:"pid,omitempty"`
	Variables map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
	Steps     []StepProgressDTO `json:"steps" yaml:"steps"`
	Surface   *SurfaceDTO       `json:"surface,omitempty" yaml:"surface,omitempty"`
}

func RunRecordDTOFrom(r *domainRuntime.Execution) *RunRecordDTO {
	if r == nil {
		return nil
	}
	steps := make([]StepProgressDTO, len(r.Steps))
	for i, sp := range r.Steps {
		steps[i] = StepProgressDTOFrom(sp)
	}
	var surface *SurfaceDTO
	if r.Surface != nil {
		surface = &SurfaceDTO{Mode: string(r.Surface.Mode), Path: r.Surface.Path, Ready: r.Surface.Ready}
	}
	return &RunRecordDTO{
		Method:    r.Method,
		PID:       r.PID,
		Variables: r.Variables,
		Steps:     steps,
		Surface:   surface,
	}
}

// SurfaceDTO is the public view of an open surface. The served directory is
// deliberately not exposed.
type SurfaceDTO struct {
	Mode  string `json:"mode" yaml:"mode"`
	Path  string `json:"path" yaml:"path"`
	Ready bool   `json:"ready" yaml:"ready"`
}
