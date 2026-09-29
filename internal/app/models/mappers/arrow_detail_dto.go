package mappers

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

func ArrowDetailDTOFrom(
	view *models.ArrowDetailView,
) *models.ArrowDetailDTO {
	if view == nil {
		return nil
	}
	return &models.ArrowDetailDTO{
		Namespace:     view.Metadata.Namespace,
		Name:          view.Metadata.Name,
		Description:   view.Metadata.Description,
		Tags:          view.Metadata.Tags,
		Variables:     view.Metadata.Variables,
		Targets:       view.Metadata.Targets,
		InstalledAt:   view.Metadata.InstalledAt,
		LastUsedAt:    view.Metadata.LastUsedAt,
		UserInstalled: view.Metadata.UserInstalled,
		Outdated:      view.Metadata.Available != nil,
		State:         view.State,
		ActiveRun:     view.ActiveRun,
		LastReturn:    view.LastReturn,
	}
}
