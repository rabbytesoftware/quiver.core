package quivers

import (
	appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"
)

// QuiverListItemDTO is the wire shape for a single item in GET /v0/quiver.
type QuiverListItemDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Removed     bool     `json:"removed"`
}

// QuiverDetailDTO is the wire shape for GET /v0/quiver/:ns.
type QuiverDetailDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url,omitempty"`
	Tags        []string `json:"tags"`
	Removed     bool     `json:"removed"`
}

func toListItemDTO(q appquiver.QuiverListDTO) QuiverListItemDTO {
	return QuiverListItemDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Name,
		Description: q.Description,
		Tags:        q.Tags,
		Removed:     q.Removed,
	}
}

func toDetailDTO(q *appquiver.QuiverDetailDTO) QuiverDetailDTO {
	return QuiverDetailDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Manifest.Name,
		Description: q.Manifest.Description,
		URL:         q.Manifest.URL,
		Tags:        q.Manifest.Tags,
		Removed:     q.Removed,
	}
}
