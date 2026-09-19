package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/models"

type CollectionListItemDTO struct {
	Namespace   string   `json:"namespace" yaml:"namespace"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Tags        []string `json:"tags" yaml:"tags"`
	ArrowCount  int      `json:"arrow_count" yaml:"arrow_count"`
	Followed    bool     `json:"followed" yaml:"followed"`
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
