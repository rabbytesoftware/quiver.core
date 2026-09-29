package dto

import "github.com/rabbytesoftware/quiver.core/internal/domain"

// AvailableDTO is a newer ref an installed row's selector points at, and the
// commit it resolves to.
type AvailableDTO struct {
	Ref    string `json:"ref" yaml:"ref"`
	Commit string `json:"commit" yaml:"commit"`
}

// AvailableDTOFrom returns nil when nothing newer is available.
func AvailableDTOFrom(a *domain.Available) *AvailableDTO {
	if a == nil {
		return nil
	}
	return &AvailableDTO{Ref: a.Ref, Commit: a.Commit}
}
