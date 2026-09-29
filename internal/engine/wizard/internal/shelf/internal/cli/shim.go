package cli

import (
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

const (
	shimOwner     = "REM quiver:"
	shimWorkdir   = "REM quiver-workdir:"
	shimExtension = ".cmd"
	shimReserved  = `"%!`
)

func shimMarker() ownership.Marker {
	return ownership.Marker{Owner: shimOwner, Workdir: shimWorkdir}
}

func shimContent(
	bare domain.Namespace,
	workdir string,
	target string,
) string {
	return "@echo off\r\n" + shimMarker().Lines(bare, workdir, "\r\n") + "\"" + target + "\" %*\r\n"
}

func placeShim(
	req models.ApplyRequest,
	c models.Candidate,
) (models.Placement, error) {
	if fsguard.UnsafePath(c.Target, shimReserved) {
		return models.Placement{Refused: models.ReasonUnsafePath}, nil
	}

	reason, err := fsguard.RequireTarget(c.Target, false)
	if err != nil || reason != "" {
		return models.Placement{Refused: reason}, err
	}

	loc := filepath.Join(req.Layout.Bin, c.Name+shimExtension)
	h, err := ownership.FileHolder(loc, shimMarker())
	if err != nil {
		return models.Placement{}, err
	}
	if refusal := h.Refusal(req.Bare); refusal != "" {
		return models.Placement{Refused: refusal}, nil
	}

	if err := fsguard.SwapFile(loc, []byte(shimContent(req.Bare, req.Workdir, c.Target))); err != nil {
		return models.Placement{}, err
	}
	return models.Placement{Location: loc}, nil
}

func removeShims(
	l platform.Layout,
	claim ownership.Claim,
	keep map[string]bool,
) error {
	return ownership.RemoveMarked(l.Bin, "", shimExtension, shimMarker(), claim, keep)
}
