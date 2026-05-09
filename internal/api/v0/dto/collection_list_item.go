package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/models"

type CollectionListItemDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	ArrowCount  int      `json:"arrow_count"`
	Followed    bool     `json:"followed"`
}

func CollectionListItemDTOFrom(q models.CollectionListDTO) CollectionListItemDTO {
	return CollectionListItemDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Name,
		Description: q.Description,
		Tags:        q.Tags,
		ArrowCount:  q.ArrowCount,
		Followed:    q.Followed,
	}
}
