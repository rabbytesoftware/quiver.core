package dto

import "github.com/rabbytesoftware/quiver/internal/domain"

type ArrowDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Tags        []string `json:"tags"`
}

func ArrowDTOFrom(
	a domain.Arrow,
) ArrowDTO {
	return ArrowDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Name,
		Version:     a.Version,
		Description: a.Description,
		Tags:        a.Tags,
	}
}
