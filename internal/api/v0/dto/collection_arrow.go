package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/models"

type CollectionArrowDTO struct {
	Namespace   string `json:"namespace"`
	Resolved    bool   `json:"resolved"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
}

func quiverArrowDTOFrom(a models.CollectionArrowDTO) CollectionArrowDTO {
	return CollectionArrowDTO{
		Namespace:   string(a.Namespace),
		Resolved:    a.Resolved,
		Name:        a.Name,
		Version:     a.Version,
		Description: a.Description,
	}
}
