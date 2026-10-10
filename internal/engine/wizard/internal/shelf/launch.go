package shelf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/launch"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

func (s *shelf) Launchable(
	_ context.Context,
	workdir string,
) bool {
	_, err := s.launchTarget(workdir)
	return err == nil
}

func (s *shelf) Launch(
	_ context.Context,
	workdir string,
) error {
	target, err := s.launchTarget(workdir)
	if err != nil {
		return fmt.Errorf("shelf: launch %s: %w", workdir, err)
	}
	if err := launch.Start(target); err != nil {
		if errors.Is(err, launch.ErrOpenFailed) {
			return fmt.Errorf("shelf: launch %s: %w: %w", workdir, ErrNotLaunchable, err)
		}
		return fmt.Errorf("shelf: launch %s: %w", workdir, err)
	}
	return nil
}

func (s *shelf) launchTarget(
	workdir string,
) (string, error) {
	l, err := s.host.Layout()
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(workdir)
	if ownership.WorkdirOwner(l.Namespaces, clean) == "" {
		return "", ErrNotAWorkdir
	}

	target, err := launch.Recorded(clean)
	if err != nil {
		return "", err
	}
	target = filepath.Clean(target)
	if target == "." || !launchAllowed(l, clean, target) {
		return "", ErrNotLaunchable
	}
	if err := launchable(target); err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotLaunchable, err)
	}
	return target, nil
}

func launchable(
	target string,
) error {
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if launch.IsBundle(target) {
		return nil
	}
	if info.IsDir() {
		return errors.New("a directory cannot be started")
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return errors.New("not executable")
	}
	return nil
}

func launchAllowed(
	l models.Layout,
	workdir string,
	target string,
) bool {
	if workfs.Inside(workdir, target) {
		return true
	}
	if !strings.EqualFold(filepath.Ext(target), models.BundleExt) {
		return false
	}
	for _, apps := range l.Apps {
		if filepath.Dir(target) == filepath.Clean(apps) {
			return true
		}
	}
	return false
}

func (s *shelf) recordLaunch(
	workdir string,
	out Applied,
) error {
	for _, e := range out.Entries {
		if e.Kind == domain.ExposeKindDesktop {
			return launch.Record(workdir, launchPath(e))
		}
	}
	return launch.Clear(workdir)
}

func launchPath(
	e AppliedEntry,
) string {
	if strings.EqualFold(filepath.Ext(e.Location), models.BundleExt) {
		return e.Location
	}
	return e.Target
}
