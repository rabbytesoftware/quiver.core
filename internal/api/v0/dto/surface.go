package dto

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

// SurfaceDTO is the public view of an open surface. The served directory is
// deliberately not exposed.
type SurfaceDTO struct {
	Mode  string `json:"mode" yaml:"mode"`
	Path  string `json:"path" yaml:"path"`
	Ready bool   `json:"ready" yaml:"ready"`
}

func SurfaceDTOFrom(
	s *domainRuntime.Surface,
) *SurfaceDTO {
	if s == nil {
		return nil
	}
	return &SurfaceDTO{
		Mode:  string(s.Mode),
		Path:  s.Path,
		Ready: s.Ready,
	}
}
