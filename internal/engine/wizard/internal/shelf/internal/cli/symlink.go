package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

func (p *placer) placeSymlink(
	ctx context.Context,
	req models.ApplyRequest,
	c models.Candidate,
) (models.Placement, error) {
	reason, err := fsguard.RequireTarget(c.Target, false)
	if err != nil || reason != "" {
		return models.Placement{Refused: reason}, err
	}
	if err := p.markDeclared(req.Workdir, c); err != nil {
		return models.Placement{}, err
	}

	loc := filepath.Join(req.Layout.Bin, c.Name)
	h, err := p.symlinkHolder(req.Layout.Namespaces, loc)
	if err != nil {
		return models.Placement{}, err
	}
	if refusal := h.Refusal(req.Bare); refusal != "" {
		return models.Placement{Refused: refusal}, nil
	}

	if err := fsguard.SwapSymlink(c.Target, loc); err != nil {
		return models.Placement{}, err
	}

	p.sign(ctx, req.Workdir, c.Target)
	return models.Placement{Location: loc}, nil
}

func (p *placer) markDeclared(
	workdir string,
	c models.Candidate,
) error {
	if !c.Declared {
		return nil
	}
	return fsguard.MarkExecutable(workdir, c.Target, p.host.Chmod)
}

func (p *placer) symlinkHolder(
	nsDir string,
	loc string,
) (ownership.Holder, error) {
	info, err := os.Lstat(loc)
	if errors.Is(err, fs.ErrNotExist) {
		return ownership.Holder{}, nil
	}
	if err != nil {
		return ownership.Holder{}, fmt.Errorf("inspect %s: %w", loc, err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return ownership.Holder{Exists: true}, nil
	}

	target, err := os.Readlink(loc)
	if err != nil {
		return ownership.Holder{}, fmt.Errorf("read link %s: %w", loc, err)
	}
	return p.linkHolder(nsDir, target), nil
}

func (p *placer) linkHolder(
	nsDir string,
	target string,
) ownership.Holder {
	if owner := ownership.WorkdirOwner(nsDir, target); owner != "" {
		return ownership.Holder{Exists: true, Namespace: owner, Target: target}
	}
	if p.host.GOOS != platform.GOOSDarwin {
		return ownership.Holder{Exists: true}
	}
	h := p.bundles.Enclosing(target)
	h.Exists = true
	return h
}

func (p *placer) removeSymlinks(
	l platform.Layout,
	claim ownership.Claim,
	keep map[string]bool,
) error {
	entries, err := os.ReadDir(l.Bin)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list %s: %w", l.Bin, err)
	}

	var errs []error
	for _, e := range entries {
		loc := filepath.Join(l.Bin, e.Name())
		if keep[loc] || e.Type()&fs.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(loc)
		if err != nil || !claim(p.linkHolder(l.Namespaces, target)) {
			continue
		}
		if err := os.Remove(loc); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", loc, err))
		}
	}
	return errors.Join(errs...)
}
