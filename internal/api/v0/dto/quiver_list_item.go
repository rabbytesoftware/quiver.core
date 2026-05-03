package dto

import "github.com/rabbytesoftware/quiver/internal/app/models"

type QuiverListItemDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	ArrowCount  int      `json:"arrow_count"`
	Followed    bool     `json:"followed"`
}

func QuiverListItemDTOFrom(q models.QuiverListDTO) QuiverListItemDTO {
	return QuiverListItemDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Name,
		Description: q.Description,
		Tags:        q.Tags,
		ArrowCount:  q.ArrowCount,
		Followed:    q.Followed,
	}
}
