package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type CollectionListDTO struct {
	Namespace   domain.Namespace `json:"namespace"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
	ArrowCount  int              `json:"arrow_count"`
	Followed    bool             `json:"followed"`
}

type CollectionDetailDTO struct {
	Namespace   domain.Namespace       `json:"namespace"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	URL         string                 `json:"url"`
	Maintainers []string               `json:"maintainers"`
	Tags        []string               `json:"tags"`
	Media       domain.CollectionMedia `json:"media"`
	Arrows      []CollectionArrowDTO   `json:"arrows"`
	Followed    bool                   `json:"followed"`
}

// CollectionArrowDTO is one member of a collection. Namespace carries the member's
// ref, so no field beside it restates which revision the collection points at.
type CollectionArrowDTO struct {
	Namespace   domain.Namespace `json:"namespace"`
	Resolved    bool             `json:"resolved"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
}
