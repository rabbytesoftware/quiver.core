package bundle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

func (e *exposer) Remove(
	_ context.Context,
	l models.Layout,
	claim models.Claim,
	keep map[string]bool,
) error {
	var errs []error
	for _, dir := range l.Apps {
		errs = append(errs, e.removeAppsIn(dir, claim, keep))
	}
	return errors.Join(errs...)
}

func (e *exposer) removeAppsIn(
	dir string,
	claim models.Claim,
	keep map[string]bool,
) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}

	var errs []error
	for _, d := range entries {
		path := filepath.Join(dir, d.Name())
		if keep[path] || !d.IsDir() || !strings.HasSuffix(d.Name(), models.BundleExt) {
			continue
		}
		if !e.bundles.Owner(path).ClaimedBy(claim) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
