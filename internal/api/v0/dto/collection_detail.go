package dto

import "github.com/rabbytesoftware/quiver/internal/app/models"

type CollectionDetailDTO struct {
	Namespace   string           `json:"namespace"`
	Name        string           `json:"name"`
	Version     string           `json:"version,omitempty"`
	Description string           `json:"description"`
	URL         string           `json:"url,omitempty"`
	Maintainers []string         `json:"maintainers"`
	Tags        []string         `json:"tags"`
	Media       CollectionMediaDTO   `json:"media,omitempty"`
	Arrows      []CollectionArrowDTO `json:"arrows"`
	Followed    bool             `json:"followed"`
}

func CollectionDetailDTOFrom(q *models.CollectionDetailDTO) CollectionDetailDTO {
	arrows := make([]CollectionArrowDTO, len(q.Arrows))
	for i, a := range q.Arrows {
		arrows[i] = quiverArrowDTOFrom(a)
	}
	return CollectionDetailDTO{
		Namespace:   string(q.Namespace),
		Name:        q.Name,
		Version:     q.Version,
		Description: q.Description,
		URL:         q.URL,
		Maintainers: q.Maintainers,
		Tags:        q.Tags,
		Media:       CollectionMediaDTO{Icon: q.Media.Icon, Banner: q.Media.Banner},
		Arrows:      arrows,
		Followed:    q.Followed,
	}
}
