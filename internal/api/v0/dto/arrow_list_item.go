package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// InstalledVersionItemDTO is one namespace@ref of a catalog entry. Ref is the
// ref the row is filed under and is always set; whether that ref is on disk is
// read from State and InstalledAt.
type InstalledVersionItemDTO struct {
	Ref         string `json:"ref"`
	State       string `json:"state"`
	InstalledAt string `json:"installed_at"`
	LastUsedAt  string `json:"last_used_at,omitempty"`
	Constraint  string `json:"constraint,omitempty"`
}

type ArrowListItemDTO struct {
	Namespace   string                    `json:"namespace"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Tags        []string                  `json:"tags"`
	Media       domain.ArrowMedia         `json:"media"`
	Versions    []InstalledVersionItemDTO `json:"versions"`
}

func ArrowListItemDTOFrom(
	a models.ArrowListDTO,
) ArrowListItemDTO {
	versions := make([]InstalledVersionItemDTO, 0, len(a.Versions))
	for _, v := range a.Versions {
		lastUsedAt := ""
		if !v.LastUsedAt.IsZero() {
			lastUsedAt = v.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		versions = append(versions, InstalledVersionItemDTO{
			Ref:         v.Ref,
			State:       string(v.State),
			InstalledAt: v.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
			LastUsedAt:  lastUsedAt,
			Constraint:  v.Constraint,
		})
	}
	return ArrowListItemDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Name,
		Description: a.Description,
		Tags:        a.Tags,
		Media:       a.Media,
		Versions:    versions,
	}
}
