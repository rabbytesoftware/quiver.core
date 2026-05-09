package dto

import "github.com/rabbytesoftware/quiver/internal/domain"

type QuiverDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func QuiverDTOFrom(q domain.Collection) QuiverDTO {
	return QuiverDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Meta.Name,
		Description: q.Meta.Description,
		Tags:        q.Meta.Tags,
	}
}
