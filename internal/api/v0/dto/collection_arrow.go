package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/models"

// CollectionArrowDTO is one member of a collection detail response. Which revision
// the collection points at is the `@ref` on Namespace, so nothing beside it says so.
type CollectionArrowDTO struct {
	Namespace   string `json:"namespace" yaml:"namespace"`
	Resolved    bool   `json:"resolved" yaml:"resolved"`
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

func quiverArrowDTOFrom(a models.CollectionArrowDTO) CollectionArrowDTO {
	return CollectionArrowDTO{
		Namespace:   string(a.Namespace),
		Resolved:    a.Resolved,
		Name:        a.Name,
		Description: a.Description,
	}
}
