package discover

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

func Declared(
	req models.Request,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	if !fsguard.SafeName(entry.Name) {
		return nil, models.ReasonUnsafeName, nil
	}

	target := fsguard.ExpandPath(entry.Path, req.Workdir)
	if !workfs.Inside(req.Workdir, target) {
		return nil, models.ReasonOutsideWorkdir, nil
	}

	reason, err := fsguard.Contained(req.Workdir, target)
	if err != nil || reason != "" {
		return nil, reason, err
	}
	return []models.Candidate{{Name: entry.Name, Target: target, Declared: true}}, "", nil
}
