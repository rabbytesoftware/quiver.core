package dto

import "github.com/rabbytesoftware/quiver/internal/domain"

type ArrowDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Tags        []string `json:"tags"`
}

func ArrowDTOFrom(a domain.Arrow) ArrowDTO {
	var meta domain.ArrowMeta
	for _, m := range a.Versions {
		meta = m.ArrowMeta
		break
	}
	return ArrowDTO{
		Namespace:   string(a.Namespace),
		Name:        meta.Name,
		Version:     meta.Version,
		Description: meta.Description,
		Tags:        meta.Tags,
	}
}
