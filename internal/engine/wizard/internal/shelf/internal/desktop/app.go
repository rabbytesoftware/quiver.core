package desktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

func bundleName(
	name string,
) string {
	if strings.HasSuffix(name, platform.BundleExt) {
		return name
	}
	return name + platform.BundleExt
}

func (p *placer) placeApp(
	ctx context.Context,
	req models.ApplyRequest,
	c models.Candidate,
) (models.Placement, error) {
	if !strings.HasSuffix(c.Target, platform.BundleExt) {
		return models.Placement{Refused: models.ReasonWrongType}, nil
	}

	name := bundleName(c.Name)
	holders, err := p.bundleHolders(req.Layout.Apps, name)
	if err != nil {
		return models.Placement{}, err
	}
	owned := ownedBundle(req.Layout.Apps, name, holders, req.Bare)

	reason, err := requireBundle(c.Target)
	if err != nil {
		return models.Placement{}, err
	}
	if reason == models.ReasonNotFound && owned != "" {
		if err := p.bundles.Tag(owned, req.Bare, req.Workdir, owned); err != nil {
			return models.Placement{}, fmt.Errorf("tag %s: %w", owned, err)
		}
		req.Moved[c.Target] = owned
		return models.Placement{Location: owned}, nil
	}
	if reason != "" {
		return models.Placement{Refused: reason}, nil
	}

	dest, refused, err := bundleDest(req, name, owned, holders)
	if err != nil || refused != "" {
		return models.Placement{Refused: refused}, err
	}

	if err := p.swapBundle(ctx, c.Target, dest, req.Bare, req.Workdir); err != nil {
		return models.Placement{}, err
	}
	req.Moved[c.Target] = dest
	return models.Placement{Location: dest}, nil
}

func requireBundle(
	target string,
) (string, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return models.ReasonNotFound, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", target, err)
	}
	if !info.IsDir() {
		return models.ReasonWrongType, nil
	}
	return "", nil
}

func (p *placer) bundleHolders(
	apps []string,
	name string,
) (map[string]ownership.Holder, error) {
	holders := map[string]ownership.Holder{}
	for _, dir := range apps {
		path := filepath.Join(dir, name)
		h, err := p.bundles.Holder(path)
		if err != nil {
			return nil, err
		}
		holders[path] = h
	}
	return holders, nil
}

func ownedBundle(
	apps []string,
	name string,
	holders map[string]ownership.Holder,
	bare domain.Namespace,
) string {
	for _, dir := range apps {
		path := filepath.Join(dir, name)
		if holders[path].Exists && holders[path].Namespace == bare {
			return path
		}
	}
	return ""
}

func bundleDest(
	req models.ApplyRequest,
	name string,
	owned string,
	holders map[string]ownership.Holder,
) (string, string, error) {
	if owned != "" {
		return owned, "", nil
	}

	dir, err := writableDir(req.Layout.Apps)
	if err != nil {
		return "", "", err
	}

	dest := filepath.Join(dir, name)
	if refusal := holders[dest].Refusal(req.Bare); refusal != "" {
		return "", refusal, nil
	}
	return dest, "", nil
}

func writableDir(
	dirs []string,
) (string, error) {
	for _, dir := range dirs {
		if probeWritable(dir) {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no writable applications directory in %v", dirs)
}

func probeWritable(
	dir string,
) bool {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".quiver-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name) == nil
}

func (p *placer) swapBundle(
	ctx context.Context,
	src string,
	dest string,
	bare domain.Namespace,
	workdir string,
) error {
	staged := dest + fsguard.StagedSuffix
	retired := dest + workfs.AsideSuffix
	for _, leftover := range []string{staged, retired} {
		if err := os.RemoveAll(leftover); err != nil {
			return fmt.Errorf("clear %s: %w", leftover, err)
		}
	}
	if err := p.host.Rename(src, staged); err != nil {
		return fmt.Errorf("move %s: %w", src, err)
	}
	if err := p.bundles.Tag(staged, bare, workdir, dest); err != nil {
		return errors.Join(fmt.Errorf("tag %s: %w", staged, err), p.host.Rename(staged, src))
	}
	if err := workfs.Swap(staged, dest, retired, p.host.Rename); err != nil {
		return errors.Join(err, p.host.Rename(staged, src))
	}
	if err := os.RemoveAll(retired); err != nil {
		slog.WarnContext(ctx, "shelf: replaced bundle left behind", "path", retired, "err", err)
	}
	return nil
}

func (p *placer) removeApps(
	l platform.Layout,
	claim ownership.Claim,
	keep map[string]bool,
) error {
	var errs []error
	for _, dir := range l.Apps {
		errs = append(errs, p.removeAppsIn(dir, claim, keep))
	}
	return errors.Join(errs...)
}

func (p *placer) removeAppsIn(
	dir string,
	claim ownership.Claim,
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
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if keep[path] || !e.IsDir() || !strings.HasSuffix(e.Name(), platform.BundleExt) {
			continue
		}
		if !claim(p.bundles.Owner(path)) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
