package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

func claim(
	workDir string,
	from string,
	to string,
) (string, bool, error) {
	source, ok := workdirRel(workDir, from)
	if _, inside := workdirRel(workDir, to); !ok || !inside || workfs.Inside(to, from) {
		return "", false, nil
	}
	if err := restoreAside(to); err != nil {
		return "", false, err
	}

	info, err := os.Lstat(to)
	if errors.Is(err, os.ErrNotExist) {
		return source, true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("portable: stat %s: %w", to, err)
	}
	if !info.IsDir() {
		return "", false, nil
	}

	owner, err := os.ReadFile(filepath.Join(to, ownerMarker)) // #nosec G304 -- marker file inside the step's own destination directory
	if err != nil || strings.TrimSpace(string(owner)) != source {
		return "", false, nil
	}

	return source, true, nil
}

func restoreAside(
	to string,
) error {
	if _, err := os.Lstat(to); !errors.Is(err, os.ErrNotExist) {
		return nil
	}

	// A crash between the swap's two renames leaves the previous install only
	// at the aside path; put it back so the destination is never left empty.
	aside := sibling(to, workfs.AsideSuffix)
	info, err := os.Lstat(aside)
	if err != nil || !info.IsDir() {
		return nil
	}

	if err := os.Rename(aside, to); err != nil {
		return fmt.Errorf("portable: restore %s: %w", to, err)
	}

	return nil
}

func (i *installer) installOwned(
	ctx context.Context,
	osArch domain.OS,
	from string,
	to string,
	name string,
	source string,
) ([]domain.PortableApp, string, error) {
	staging := sibling(to, stagingSuffix)
	aside := sibling(to, workfs.AsideSuffix)
	for _, leftover := range []string{staging, aside} {
		if err := os.RemoveAll(leftover); err != nil {
			return nil, "", fmt.Errorf("portable: remove leftover %s: %w", leftover, err)
		}
	}

	apps, output, err := i.install(ctx, osArch, from, staging, name, true)
	if err == nil {
		err = markOwned(staging, source)
	}
	if err == nil {
		err = workfs.Swap(staging, to, aside, os.Rename)
	}
	if err != nil {
		_ = os.RemoveAll(staging)
		return nil, "", fmt.Errorf("portable: %w", err)
	}
	_ = os.RemoveAll(aside)

	return relocateApps(apps, staging, to), workfs.Relocate(output, staging, to), nil
}

func sibling(
	path string,
	suffix string,
) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+suffix)
}

func markOwned(
	dir string,
	source string,
) error {
	marker := filepath.Join(dir, ownerMarker)
	if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("portable: mark %s: %w", dir, err)
	}

	if err := os.WriteFile(marker, []byte(source+"\n"), ownerMarkerPerm); err != nil {
		return fmt.Errorf("portable: mark %s: %w", dir, err)
	}

	return nil
}

func relocateApps(
	apps []domain.PortableApp,
	staging string,
	to string,
) []domain.PortableApp {
	moved := make([]domain.PortableApp, 0, len(apps))
	for _, app := range apps {
		moved = append(moved, domain.PortableApp{
			Name:  app.Name,
			Entry: workfs.Relocate(app.Entry, staging, to),
			Icon:  workfs.Relocate(app.Icon, staging, to),
		})
	}

	return moved
}
