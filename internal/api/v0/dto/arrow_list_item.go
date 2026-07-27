package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// InstalledVersionItemDTO is one namespace@ref of a catalog entry. Version is
// the ref the row is filed under and is always set; Ref is the stamp a
// successful _install leaves and an _uninstall takes back off, so a row that is
// in the catalog but not on disk carries a Version and an empty Ref.
type InstalledVersionItemDTO struct {
	Ref         string `json:"ref"`
	Version     string `json:"version"`
	State       string `json:"state"`
	InstalledAt string `json:"installed_at"`
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
		versions = append(versions, InstalledVersionItemDTO{
			Ref:         v.Ref,
			Version:     v.Version,
			State:       string(v.State),
			InstalledAt: v.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
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
