package dto

import appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"

type QuiverListItemDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func QuiverListItemDTOFrom(q appquiver.QuiverListDTO) QuiverListItemDTO {
	return QuiverListItemDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Name,
		Description: q.Description,
		Tags:        q.Tags,
	}
}
