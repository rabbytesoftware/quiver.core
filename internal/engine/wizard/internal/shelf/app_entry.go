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
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

const appMarker = ".quiver-app"

func (s *shelf) AppEntry(
	_ context.Context,
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

	data, err := os.ReadFile(filepath.Join(clean, appMarker)) // #nosec G304 -- the shelf's own marker inside a validated workdir
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoApp, err)
	}
	target := filepath.Clean(strings.TrimSpace(string(data)))
	if !appAllowed(l, clean, target) {
		return "", fmt.Errorf("%w: %s is outside the arrow", ErrNoApp, target)
	}
	exe, err := appExecutable(target)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoApp, err)
	}
	return exe, nil
}

func (s *shelf) recordApp(
	workdir string,
	out Applied,
) error {
	marker := filepath.Join(workdir, appMarker)
	for _, e := range out.Entries {
		if e.Kind == domain.ExposeKindDesktop {
			return os.WriteFile(marker, []byte(appPath(e)+"\n"), 0o600)
		}
	}
	if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func appPath(
	e AppliedEntry,
) string {
	if strings.EqualFold(filepath.Ext(e.Location), models.BundleExt) {
		return e.Location
	}
	return e.Target
}

func appAllowed(
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

func appExecutable(
	target string,
) (string, error) {
	if strings.EqualFold(filepath.Ext(target), models.BundleExt) {
		return bundleExecutable(target)
	}
	info, err := os.Stat(target) // #nosec G703 -- target was checked against the arrow's own directories by appAllowed
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("a directory cannot be started")
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return "", errors.New("not executable")
	}
	return target, nil
}

func bundleExecutable(
	bundle string,
) (string, error) {
	dir := filepath.Join(bundle, "Contents", "MacOS")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	stem := strings.TrimSuffix(filepath.Base(bundle), filepath.Ext(bundle))
	var only string
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(e.Name(), stem) {
			return filepath.Join(dir, e.Name()), nil
		}
		only = filepath.Join(dir, e.Name())
		count++
	}
	if count != 1 {
		return "", fmt.Errorf("%s has no single executable to start", bundle)
	}
	return only, nil
}
