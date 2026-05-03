package models

import "github.com/rabbytesoftware/quiver/internal/domain"

type QuiverListDTO struct {
	Namespace   domain.Namespace `json:"namespace"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
	ArrowCount  int              `json:"arrow_count"`
	Followed    bool             `json:"followed"`
}

type QuiverDetailDTO struct {
	Namespace   domain.Namespace `json:"namespace"`
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description"`
	URL         string           `json:"url"`
	Maintainers []string         `json:"maintainers"`
	Tags        []string         `json:"tags"`
	Media       domain.QuiverMedia `json:"media"`
	Arrows      []QuiverArrowDTO `json:"arrows"`
	Followed    bool             `json:"followed"`
}

type QuiverArrowDTO struct {
	Namespace   domain.Namespace `json:"namespace"`
	Resolved    bool             `json:"resolved"`
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description"`
}
