package shelf

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
)

const (
	bundleExtension = ".app"
	retiredSuffix   = ".quiver-old"
)

func bundleName(
	name string,
) string {
	if strings.HasSuffix(name, bundleExtension) {
		return name
	}
	return name + bundleExtension
}

func (s *shelf) placeApp(
	ctx context.Context,
	req applyRequest,
	c candidate,
) (placement, error) {
	if !strings.HasSuffix(c.target, bundleExtension) {
		return placement{refused: reasonWrongType}, nil
	}

	name := bundleName(c.name)
	holders, err := s.bundleHolders(req.layout.apps, name)
	if err != nil {
		return placement{}, err
	}
	owned := ownedBundle(req.layout.apps, name, holders, req.bare)

	reason, err := requireBundle(c.target)
	if err != nil {
		return placement{}, err
	}
	if reason == reasonNotFound && owned != "" {
		req.moved[c.target] = owned
		return placement{location: owned}, nil
	}
	if reason != "" {
		return placement{refused: reason}, nil
	}

	dest, refused, err := bundleDest(req, name, owned, holders)
	if err != nil || refused != "" {
		return placement{refused: refused}, err
	}

	if err := s.swapBundle(ctx, c.target, dest, req.bare); err != nil {
		return placement{}, err
	}
	req.moved[c.target] = dest
	return placement{location: dest}, nil
}

func requireBundle(
	target string,
) (string, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return reasonNotFound, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", target, err)
	}
	if !info.IsDir() {
		return reasonWrongType, nil
	}
	return "", nil
}

func (s *shelf) bundleHolders(
	apps []string,
	name string,
) (map[string]holder, error) {
	holders := map[string]holder{}
	for _, dir := range apps {
		path := filepath.Join(dir, name)
		h, err := s.bundleHolder(path)
		if err != nil {
			return nil, err
		}
		holders[path] = h
	}
	return holders, nil
}

func (s *shelf) bundleHolder(
	path string,
) (holder, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return holder{}, nil
	}
	if err != nil {
		return holder{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.IsDir() {
		return holder{exists: true}, nil
	}
	return holder{exists: true, namespace: s.bundleOwner(path)}, nil
}

func (s *shelf) bundleOwner(
	path string,
) domain.Namespace {
	tag, err := s.tagger.read(path)
	if err != nil {
		return ""
	}
	return bundleTagOwner(tag, path)
}

func bundleTag(
	bare domain.Namespace,
	bundle string,
) string {
	return bare.String() + "\n" + filepath.Base(bundle)
}

func bundleTagOwner(
	tag string,
	bundle string,
) domain.Namespace {
	owner, name, ok := strings.Cut(tag, "\n")
	if !ok || name != filepath.Base(bundle) {
		return ""
	}
	return domain.Namespace(owner)
}

func ownedBundle(
	apps []string,
	name string,
	holders map[string]holder,
	bare domain.Namespace,
) string {
	for _, dir := range apps {
		path := filepath.Join(dir, name)
		if holders[path].exists && holders[path].namespace == bare {
			return path
		}
	}
	return ""
}

func bundleDest(
	req applyRequest,
	name string,
	owned string,
	holders map[string]holder,
) (string, string, error) {
	if owned != "" {
		return owned, "", nil
	}

	dir, err := writableDir(req.layout.apps)
	if err != nil {
		return "", "", err
	}

	dest := filepath.Join(dir, name)
	if refusal := holders[dest].refusal(req.bare); refusal != "" {
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

func (s *shelf) swapBundle(
	ctx context.Context,
	src string,
	dest string,
	bare domain.Namespace,
) error {
	staged := dest + stagedSuffix
	retired := dest + retiredSuffix
	if err := clearPaths(staged, retired); err != nil {
		return err
	}
	if err := s.rename(src, staged); err != nil {
		return fmt.Errorf("move %s: %w", src, err)
	}
	if err := s.tagger.write(staged, bundleTag(bare, dest)); err != nil {
		return errors.Join(fmt.Errorf("tag %s: %w", staged, err), s.rename(staged, src))
	}
	if err := s.renameIfExists(dest, retired); err != nil {
		return errors.Join(fmt.Errorf("retire %s: %w", dest, err), s.rename(staged, src))
	}
	if err := s.rename(staged, dest); err != nil {
		return errors.Join(fmt.Errorf("swap %s: %w", dest, err), s.renameIfExists(retired, dest), s.rename(staged, src))
	}
	if err := os.RemoveAll(retired); err != nil {
		slog.WarnContext(ctx, "shelf: replaced bundle left behind", "path", retired, "err", err)
	}
	return nil
}

func (s *shelf) renameIfExists(
	from string,
	to string,
) error {
	err := s.rename(from, to)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func clearPaths(
	paths ...string,
) error {
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("clear %s: %w", p, err)
		}
	}
	return nil
}

func (s *shelf) enclosingBundleOwner(
	target string,
) domain.Namespace {
	for dir := filepath.Dir(target); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if !strings.HasSuffix(dir, bundleExtension) {
			continue
		}
		if owner := s.bundleOwner(dir); owner != "" {
			return owner
		}
	}
	return ""
}

func (s *shelf) removeApps(
	l layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	var errs []error
	for _, dir := range l.apps {
		errs = append(errs, s.removeAppsIn(dir, bare, keep))
	}
	return errors.Join(errs...)
}

func (s *shelf) removeAppsIn(
	dir string,
	bare domain.Namespace,
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
		if keep[path] || !e.IsDir() || !strings.HasSuffix(e.Name(), bundleExtension) {
			continue
		}
		if s.bundleOwner(path) != bare {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
