package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
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
	return p.markExecutable(workdir, c.Target)
}

func (p *placer) markExecutable(
	workdir string,
	target string,
) error {
	resolved, ok := fsguard.ResolveInside(workdir, target)
	if !ok {
		return nil
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", resolved, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
		return nil
	}
	if err := p.host.Chmod(resolved, info.Mode().Perm()|0o111); err != nil {
		return fmt.Errorf("mark %s executable: %w", resolved, err)
	}
	return nil
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
	return ownership.Holder{Exists: true, Namespace: p.linkOwner(nsDir, target)}, nil
}

func (p *placer) linkOwner(
	nsDir string,
	target string,
) domain.Namespace {
	if owner := ownership.WorkdirOwner(nsDir, target); owner != "" {
		return owner
	}
	if p.host.GOOS != platform.GOOSDarwin {
		return ""
	}
	return p.bundles.Enclosing(target)
}

func (p *placer) removeSymlinks(
	l platform.Layout,
	bare domain.Namespace,
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
		if err != nil || p.linkOwner(l.Namespaces, target) != bare {
			continue
		}
		if err := os.Remove(loc); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", loc, err))
		}
	}
	return errors.Join(errs...)
}
