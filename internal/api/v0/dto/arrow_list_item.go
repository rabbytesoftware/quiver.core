package dto

import "github.com/rabbytesoftware/quiver/internal/app/arrow"

type ArrowListItemDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Tags        []string `json:"tags"`
}

func ArrowListItemDTOFrom(a arrow.ArrowListDTO) ArrowListItemDTO {
	return ArrowListItemDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Name,
		Version:     a.Version,
		Description: a.Description,
		State:       string(a.State),
		Tags:        a.Tags,
	}
}
