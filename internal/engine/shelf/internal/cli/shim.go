package cli

import (
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

const (
	shimMarker    = "REM quiver:"
	shimExtension = ".cmd"
	shimReserved  = `"%!`
)

func shimContent(
	bare domain.Namespace,
	target string,
) string {
	return "@echo off\r\n" + shimMarker + string(bare) + "\r\n\"" + target + "\" %*\r\n"
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
	h, err := ownership.FileHolder(loc, shimMarker)
	if err != nil {
		return models.Placement{}, err
	}
	if refusal := h.Refusal(req.Bare); refusal != "" {
		return models.Placement{Refused: refusal}, nil
	}

	if err := fsguard.SwapFile(loc, []byte(shimContent(req.Bare, c.Target))); err != nil {
		return models.Placement{}, err
	}
	return models.Placement{Location: loc}, nil
}

func removeShims(
	l platform.Layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	return ownership.RemoveMarked(l.Bin, "", shimExtension, shimMarker, bare, keep)
}
