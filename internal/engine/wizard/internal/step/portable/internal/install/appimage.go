package install

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
)

func (i *installer) installAppImage(
	ctx context.Context,
	src *os.File,
	size int64,
	from string,
	to string,
	staged bool,
) ([]domain.PortableApp, error) {
	name := unpack.AppDirName(from)
	appDir, staging, err := appDirPaths(to, name)
	if err != nil {
		return nil, err
	}
	if staged {
		staging = appDir
	}

	if err := os.RemoveAll(staging); err != nil {
		return nil, fmt.Errorf("portable: remove leftover %s: %w", staging, err)
	}

	meta, err := i.stageAppImage(ctx, src, size, staging)
	if err == nil && !staged {
		err = swapAppDir(staging, appDir)
	}
	if err != nil {
		_ = os.RemoveAll(staging)
		return nil, err
	}

	return []domain.PortableApp{{
		Name:  cmp.Or(meta.Name, name),
		Entry: filepath.Join(appDir, unpack.LauncherName),
		Icon:  appImageIcon(appDir, meta.Icon),
	}}, nil
}

func (i *installer) stageAppImage(
	ctx context.Context,
	src *os.File,
	size int64,
	staging string,
) (unpack.AppImageMeta, error) {
	g, err := unpack.OpenGuard(ctx, staging, i.maxBytes, unpack.SkipEscapingLinks())
	if err != nil {
		return unpack.AppImageMeta{}, err
	}
	defer g.Close()

	if err := errors.Join(unpack.ExtractAppImage(ctx, src, size, g), g.Verify()); err != nil {
		return unpack.AppImageMeta{}, err
	}

	meta, err := unpack.ReadAppImageMeta(staging)
	if err != nil {
		return unpack.AppImageMeta{}, err
	}

	if _, err := unpack.WriteLauncher(staging, meta.Args); err != nil {
		return unpack.AppImageMeta{}, err
	}

	return meta, nil
}

func swapAppDir(
	staging string,
	appDir string,
) error {
	if err := removeOwnedAppDir(appDir); err != nil {
		return err
	}

	if err := os.Rename(staging, appDir); err != nil {
		return fmt.Errorf("portable: move %s into place: %w", appDir, err)
	}

	return nil
}

func removeOwnedAppDir(
	appDir string,
) error {
	_, err := os.Lstat(appDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("portable: stat %s: %w", appDir, err)
	}

	if _, err := os.Lstat(filepath.Join(appDir, unpack.LauncherName)); err != nil {
		return fmt.Errorf("%w: %s has no %s", ErrUnownedAppDir, appDir, unpack.LauncherName)
	}

	if err := os.RemoveAll(appDir); err != nil {
		return fmt.Errorf("portable: remove previous %s: %w", appDir, err)
	}

	return nil
}

func appImageIcon(
	appDir string,
	icon string,
) string {
	if icon == "" {
		return ""
	}

	return filepath.Join(appDir, filepath.FromSlash(icon))
}

func appDirPaths(
	to string,
	name string,
) (string, string, error) {
	parent := filepath.Clean(to)
	appDir := filepath.Join(to, name)
	staging := filepath.Join(to, "."+name+stagingSuffix)
	if name == "." || name == ".." || filepath.Dir(appDir) != parent || filepath.Dir(staging) != parent {
		return "", "", fmt.Errorf("portable: appdir %q is not a direct child of %s", name, to)
	}

	return appDir, staging, nil
}
