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
		Name:        q.Name,
		Description: q.Description,
		URL:         q.URL,
		Tags:        q.Tags,
	}
}
