package mappers

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func ArrowManifestDTOFrom(
	arrow *domain.Arrow,
) *models.ArrowManifestDTO {
	if arrow == nil {
		return nil
	}
	return &models.ArrowManifestDTO{
		Namespace:   arrow.Namespace,
		Name:        arrow.Name,
		Description: arrow.Description,
		Tags:        arrow.Tags,
		Variables:   arrow.Variables,
		Targets:     arrow.Targets,
		Manifest:    arrow,
	}
}
