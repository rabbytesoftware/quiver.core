package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
)

type symlink struct {
	host      host.Host
	scan      discover.Scan
	signer    Signer
	enclosing func(target string) ownership.Holder
}

func NewSymlink(
	h host.Host,
	scan discover.Scan,
	signer Signer,
	enclosing func(target string) ownership.Holder,
) models.Exposer {
	return &symlink{host: h, scan: scan, signer: signer, enclosing: enclosing}
}

func (s *symlink) Find(
	req models.Request,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	return discover.Commands(req, entry, s.scan)
}

func (s *symlink) Place(
	ctx context.Context,
	req models.Request,
	_ domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	reason, err := fsguard.RequireTarget(c.Target, false)
	if err != nil || reason != "" {
		return models.Placement{Refused: reason}, err
	}
	if err := s.markDeclared(req.Workdir, c); err != nil {
		return models.Placement{}, err
	}

	loc := filepath.Join(req.Layout.Bin, c.Name)
	h, err := s.holder(req.Layout.Namespaces, loc)
	if err != nil {
		return models.Placement{}, err
	}
	if refusal := h.Refusal(req.Bare); refusal != "" {
		return models.Placement{Refused: refusal}, nil
	}

	if err := fsguard.SwapSymlink(c.Target, loc); err != nil {
		return models.Placement{}, err
	}

	s.signer.Sign(ctx, req.Workdir, c.Target)
	return models.Placement{Location: loc}, nil
}

func (s *symlink) Remove(
	_ context.Context,
	l models.Layout,
	claim models.Claim,
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
		if err != nil || !s.linkHolder(l.Namespaces, target).ClaimedBy(claim) {
			continue
		}
		if err := os.Remove(loc); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", loc, err))
		}
	}
	return errors.Join(errs...)
}

func (s *symlink) markDeclared(
	workdir string,
	c models.Candidate,
) error {
	if !c.Declared {
		return nil
	}
	return fsguard.MarkExecutable(workdir, c.Target, s.host.Chmod)
}

func (s *symlink) holder(
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
	return s.linkHolder(nsDir, target), nil
}

func (s *symlink) linkHolder(
	nsDir string,
	target string,
) ownership.Holder {
	if owner := ownership.WorkdirOwner(nsDir, target); owner != "" {
		return ownership.Holder{Exists: true, Namespace: owner, Target: target}
	}
	h := s.enclosing(target)
	h.Exists = true
	return h
}
