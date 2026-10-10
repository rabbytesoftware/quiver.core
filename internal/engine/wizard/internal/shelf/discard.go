package shelf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
)

func (s *shelf) Discard(
	_ context.Context,
	workdir string,
) error {
	l, err := s.host.Layout()
	if err != nil {
		return fmt.Errorf("shelf: discard %s: %w", workdir, err)
	}
	clean := filepath.Clean(workdir)
	if ownership.WorkdirOwner(l.Namespaces, clean) == "" {
		return fmt.Errorf("shelf: discard %s: %w", workdir, ErrNotAWorkdir)
	}

	entries, err := os.ReadDir(clean)
	if err != nil {
		return fmt.Errorf("shelf: discard %s: %w", workdir, err)
	}
	var failed error
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(clean, e.Name())); err != nil && failed == nil {
			failed = err
		}
	}
	if failed != nil {
		return fmt.Errorf("shelf: discard %s: %w", workdir, failed)
	}
	return nil
}
