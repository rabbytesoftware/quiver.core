package xdg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
)

type exposer struct {
	host host.Host
	scan discover.Scan
}

func New(
	h host.Host,
	scan discover.Scan,
) models.Exposer {
	return &exposer{host: h, scan: scan}
}

func (e *exposer) Place(
	_ context.Context,
	req models.Request,
	entry domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	if fsguard.UnsafePath(c.Target, reserved) {
		return models.Placement{Refused: models.ReasonUnsafePath}, nil
	}

	reason, err := fsguard.RequireTarget(c.Target, false)
	if err != nil || reason != "" {
		return models.Placement{Refused: reason}, err
	}

	dir := entriesDir(req.Layout.UserHome)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- the XDG applications dir is shared and must be traversable
		return models.Placement{}, fmt.Errorf("create %s: %w", dir, err)
	}

	loc := filepath.Join(dir, fileName(req.Bare, c.Name))
	h, err := ownership.FileHolder(loc, marker())
	if err != nil {
		return models.Placement{}, err
	}
	if refusal := h.Refusal(req.Bare); refusal != "" {
		return models.Placement{Refused: refusal}, nil
	}

	if err := e.markAppImage(req.Workdir, c.Target); err != nil {
		return models.Placement{}, err
	}

	data := content(req.Bare, req.Workdir, c.DisplayName(), c.Target, discover.Icon(req, entry, c), validCategories(entry.Categories))
	if err := fsguard.SwapFile(loc, []byte(data)); err != nil {
		return models.Placement{}, err
	}
	return models.Placement{Location: loc}, nil
}

func (e *exposer) Remove(
	_ context.Context,
	l models.Layout,
	claim models.Claim,
	keep map[string]bool,
) error {
	return ownership.RemoveMarked(entriesDir(l.UserHome), filePrefix, fileExt, marker(), claim, keep)
}

func (e *exposer) markAppImage(
	workdir string,
	target string,
) error {
	if !fsguard.HasSuffixFold(target, models.AppImageExt) {
		return nil
	}
	return fsguard.MarkExecutable(workdir, target, e.host.Chmod)
}
