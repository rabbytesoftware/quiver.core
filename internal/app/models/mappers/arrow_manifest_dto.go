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
		Manifest:    stripRefMutability(arrow),
	}
}

// stripRefMutability clears the fields that mark a ref as resolved onto a
// branch. They are Quiver's own bookkeeping, not the client's: the Manifest
// field otherwise embeds *domain.Arrow verbatim, and that struct is where the
// resolver stamps them, so without this the manifest response would leak
// them straight from storage.
func stripRefMutability(
	arrow *domain.Arrow,
) *domain.Arrow {
	stripped := *arrow
	stripped.RefIsBranch = false
	stripped.RefCommitSHA = ""
	return &stripped
}
