package dto

import "github.com/rabbytesoftware/quiver/internal/app/models"

type QuiverDetailDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url,omitempty"`
	Tags        []string `json:"tags"`
}

func QuiverDetailDTOFrom(q *models.QuiverDetailDTO) QuiverDetailDTO {
	return QuiverDetailDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Manifest.Name,
		Description: q.Manifest.Description,
		URL:         q.Manifest.URL,
		Tags:        q.Manifest.Tags,
	}
}
