package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ArrowManifestDTO struct {
	Namespace   string                      `json:"namespace" yaml:"namespace"`
	Name        string                      `json:"name" yaml:"name"`
	Description string                      `json:"description" yaml:"description"`
	Tags        []string                    `json:"tags" yaml:"tags"`
	Variables   []domain.Variable           `json:"variables" yaml:"variables"`
	Targets     map[domain.OS]domain.Target `json:"targets" yaml:"targets"`
	Manifest    *domain.Arrow               `json:"manifest" yaml:"manifest"`
}

func ArrowManifestDTOFrom(a *models.ArrowManifestDTO) *ArrowManifestDTO {
	if a == nil {
		return nil
	}
	return &ArrowManifestDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Name,
		Description: a.Description,
		Tags:        a.Tags,
		Variables:   a.Variables,
		Targets:     a.Targets,
		Manifest:    a.Manifest,
	}
}
