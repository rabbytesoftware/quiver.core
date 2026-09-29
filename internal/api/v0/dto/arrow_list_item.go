package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// InstalledVersionItemDTO is one namespace@selector of a catalog entry. Ref is
// the selector the row is filed under and is always set; ResolvedRef is the ref
// it installed, empty until it resolved one.
type InstalledVersionItemDTO struct {
	Ref         string `json:"ref" yaml:"ref"`
	ResolvedRef string `json:"resolved_ref" yaml:"resolved_ref"`
	State       string `json:"state" yaml:"state"`
	InstalledAt string `json:"installed_at" yaml:"installed_at"`
	LastUsedAt  string `json:"last_used_at,omitempty" yaml:"last_used_at,omitempty"`
}

type ArrowListItemDTO struct {
	Namespace   string                    `json:"namespace" yaml:"namespace"`
	Name        string                    `json:"name" yaml:"name"`
	Description string                    `json:"description" yaml:"description"`
	Tags        []string                  `json:"tags" yaml:"tags"`
	Media       domain.ArrowMedia         `json:"media" yaml:"media"`
	Versions    []InstalledVersionItemDTO `json:"versions" yaml:"versions"`
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
			ResolvedRef: v.ResolvedRef,
			State:       string(v.State),
			InstalledAt: v.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
			LastUsedAt:  lastUsedAt,
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
