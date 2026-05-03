package dto

import (
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type ArrowManifestDTO struct {
	Namespace   string                      `json:"namespace"`
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Version     string                      `json:"version"`
	Tags        []string                    `json:"tags"`
	Variables   []domain.Variable           `json:"variables"`
	Targets     map[domain.OS]domain.Target `json:"targets"`
	Manifest    *domain.Arrow               `json:"manifest"`
}

func ArrowManifestDTOFrom(a *models.ArrowManifestDTO) *ArrowManifestDTO {
	if a == nil {
		return nil
	}
	return &ArrowManifestDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Name,
		Description: a.Description,
		Version:     a.Version,
		Tags:        a.Tags,
		Variables:   a.Variables,
		Targets:     a.Targets,
		Manifest:    a.Manifest,
	}
}
