package cli

import (
	"context"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/userpath"
)

// A PATH list entry can neither hold its separator nor a quote, and a `%` would
// be expanded inside the REG_EXPAND_SZ user Path.
const pathDirReserved = `;"%`

type pathDir struct {
	scan   discover.Scan
	editor userpath.Editor
}

func NewPathDir(
	scan discover.Scan,
	editor userpath.Editor,
) models.Exposer {
	return &pathDir{scan: scan, editor: editor}
}

func (p *pathDir) Find(
	req models.Request,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	return discover.Commands(req, entry, p.scan)
}

func (p *pathDir) Place(
	ctx context.Context,
	req models.Request,
	_ domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	reason, err := p.refusal(req.Workdir, c.Target)
	if err != nil || reason != "" {
		return models.Placement{Refused: reason}, err
	}

	dir := filepath.Dir(c.Target)
	if err := p.editor.Append(ctx, dir); err != nil {
		return models.Placement{}, err
	}
	return models.Placement{Location: dir}, nil
}

func (p *pathDir) Remove(
	ctx context.Context,
	l models.Layout,
	claim models.Claim,
	keep map[string]bool,
) error {
	return p.editor.Drop(ctx, func(entry string) bool {
		return claimedDir(l.Namespaces, filepath.Clean(entry), claim, keep)
	})
}

func (p *pathDir) refusal(
	workdir string,
	target string,
) (string, error) {
	if fsguard.UnsafePath(target, pathDirReserved) {
		return models.ReasonUnsafePath, nil
	}

	reason, err := fsguard.RequireTarget(target, false)
	if err != nil || reason != "" {
		return reason, err
	}
	if !fsguard.HasSuffixFold(target, models.ExeExt) {
		return models.ReasonWrongType, nil
	}
	if _, ok := fsguard.ResolveInside(workdir, target); !ok {
		return models.ReasonOutsideWorkdir, nil
	}
	if !fsguard.SafeName(p.scan.Rule.Stem(filepath.Base(target))) {
		return models.ReasonUnsafeName, nil
	}
	return "", nil
}

func claimedDir(
	nsDir string,
	dir string,
	claim models.Claim,
	keep map[string]bool,
) bool {
	for loc := range keep {
		if userpath.Same(loc, dir, true) {
			return false
		}
	}

	owner := ownership.WorkdirOwner(nsDir, dir)
	if owner == "" {
		return false
	}
	return ownership.Holder{Exists: true, Namespace: owner, Target: dir}.ClaimedBy(claim)
}
