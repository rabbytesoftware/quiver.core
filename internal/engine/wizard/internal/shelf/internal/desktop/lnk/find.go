package lnk

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

func (e *exposer) Find(
	req models.Request,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	return discover.Launchers(req, entry, e.scan, models.ExeExt)
}
