package shelf

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func (s *shelf) placeSymlink(
	ctx context.Context,
	req applyRequest,
	c candidate,
) (placement, error) {
	reason, err := requireTarget(c.target, false)
	if err != nil || reason != "" {
		return placement{refused: reason}, err
	}
	if err := s.markDeclared(req.workdir, c); err != nil {
		return placement{}, err
	}

	loc := filepath.Join(req.layout.bin, c.name)
	h, err := s.symlinkHolder(req.layout.namespaces, loc)
	if err != nil {
		return placement{}, err
	}
	if refusal := h.refusal(req.bare); refusal != "" {
		return placement{refused: refusal}, nil
	}

	if err := swapSymlink(c.target, loc); err != nil {
		return placement{}, err
	}

	s.sign(ctx, req.workdir, c.target)
	return placement{location: loc}, nil
}

func (s *shelf) markDeclared(
	workdir string,
	c candidate,
) error {
	if !c.declared {
		return nil
	}
	return s.markExecutable(workdir, c.target)
}

func (s *shelf) markExecutable(
	workdir string,
	target string,
) error {
	resolved, ok := resolveInside(workdir, target)
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
	if err := s.chmod(resolved, info.Mode().Perm()|0o111); err != nil {
		return fmt.Errorf("mark %s executable: %w", resolved, err)
	}
	return nil
}

func (s *shelf) symlinkHolder(
	nsDir string,
	loc string,
) (holder, error) {
	info, err := os.Lstat(loc)
	if errors.Is(err, fs.ErrNotExist) {
		return holder{}, nil
	}
	if err != nil {
		return holder{}, fmt.Errorf("inspect %s: %w", loc, err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return holder{exists: true}, nil
	}

	target, err := os.Readlink(loc)
	if err != nil {
		return holder{}, fmt.Errorf("read link %s: %w", loc, err)
	}
	return holder{exists: true, namespace: s.linkOwner(nsDir, target)}, nil
}

func (s *shelf) linkOwner(
	nsDir string,
	target string,
) domain.Namespace {
	if owner := workdirOwner(nsDir, target); owner != "" {
		return owner
	}
	if s.goos != goosDarwin {
		return ""
	}
	return s.enclosingBundleOwner(target)
}

func (s *shelf) removeSymlinks(
	l layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	entries, err := os.ReadDir(l.bin)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list %s: %w", l.bin, err)
	}

	var errs []error
	for _, e := range entries {
		loc := filepath.Join(l.bin, e.Name())
		if keep[loc] || e.Type()&fs.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(loc)
		if err != nil || s.linkOwner(l.namespaces, target) != bare {
			continue
		}
		if err := os.Remove(loc); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", loc, err))
		}
	}
	return errors.Join(errs...)
}
