package mappers

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

func ArrowListDTOsFrom(
	views []models.ArrowView,
) []models.ArrowListDTO {
	result := make([]models.ArrowListDTO, 0, len(views))
	for _, v := range views {
		vDTOs := make([]models.InstalledVersionDTO, 0, len(v.Versions))
		for _, ver := range v.Versions {
			vDTOs = append(vDTOs, models.InstalledVersionDTO{
				Ref:         ver.Metadata.InstalledRef,
				Version:     ver.Metadata.Version,
				State:       ver.State,
				InstalledAt: ver.Metadata.InstalledAt,
				Constraint:  ver.Metadata.InstalledConstraint,
			})
		}
		result = append(result, models.ArrowListDTO{
			Namespace:   v.Namespace,
			Name:        v.Metadata.Name,
			Description: v.Metadata.Description,
			Tags:        v.Metadata.Tags,
			Versions:    vDTOs,
		})
	}
	return result
}
