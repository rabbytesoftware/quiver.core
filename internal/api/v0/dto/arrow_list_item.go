package dto

import "github.com/rabbytesoftware/quiver/internal/app/models"

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
		Versions:    versions,
	}
}
