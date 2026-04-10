package dto

import appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"

type QuiverDetailDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url,omitempty"`
	Tags        []string `json:"tags"`
	Removed     bool     `json:"removed"`
}

func QuiverDetailDTOFrom(q *appquiver.QuiverDetailDTO) QuiverDetailDTO {
	return QuiverDetailDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Manifest.Name,
		Description: q.Manifest.Description,
		URL:         q.Manifest.URL,
		Tags:        q.Manifest.Tags,
		Removed:     q.Removed,
	}
}
