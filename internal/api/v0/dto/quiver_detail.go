package dto

import "github.com/rabbytesoftware/quiver/internal/app/models"

type QuiverDetailDTO struct {
	Namespace   string           `json:"namespace"`
	Name        string           `json:"name"`
	Version     string           `json:"version,omitempty"`
	Description string           `json:"description"`
	URL         string           `json:"url,omitempty"`
	Maintainers []string         `json:"maintainers"`
	Tags        []string         `json:"tags"`
	Media       QuiverMediaDTO   `json:"media,omitempty"`
	Arrows      []QuiverArrowDTO `json:"arrows"`
	Followed    bool             `json:"followed"`
}

func QuiverDetailDTOFrom(q *models.QuiverDetailDTO) QuiverDetailDTO {
	arrows := make([]QuiverArrowDTO, len(q.Arrows))
	for i, a := range q.Arrows {
		arrows[i] = quiverArrowDTOFrom(a)
	}
	return QuiverDetailDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Name,
		Version:     q.Version,
		Description: q.Description,
		URL:         q.URL,
		Maintainers: q.Maintainers,
		Tags:        q.Tags,
		Media:       QuiverMediaDTO{Icon: q.Media.Icon, Banner: q.Media.Banner},
		Arrows:      arrows,
		Followed:    q.Followed,
	}
}
