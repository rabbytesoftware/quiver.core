package dest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

const (
	ownerMarker     = ".quiver-portable"
	ownerMarkerPerm = 0o644
)

var ErrUnowned = errors.New("portable: directory not managed by quiver")

type Build func(
	dir string,
) (unpack.Result, error)

type Destination interface {
	Stage(
		unit unpack.Unit,
		build Build,
	) (unpack.Result, error)
}

type destination struct {
	path   string
	source string
	owned  bool
}

// Claim decides whether the step owns to: only a destination inside the
// workdir that is absent, or that carries this source's owner marker, is ever
// replaced wholesale; anything else is merged into.
func Claim(
	workDir string,
	from string,
	to string,
) (Destination, error) {
	d := &destination{path: to}
	source, ok := WorkdirRel(workDir, from)
	if _, inside := WorkdirRel(workDir, to); !ok || !inside || workfs.Inside(to, from) {
		return d, nil
	}
	if err := restoreAside(to); err != nil {
		return nil, err
	}

	owned, err := ownedBy(to, source)
	if err != nil {
		return nil, err
	}
	d.source, d.owned = source, owned

	return d, nil
}

func (d *destination) Stage(
	unit unpack.Unit,
	build Build,
) (unpack.Result, error) {
	if d.owned {
		return stageOwned(d.path, d.source, unit, build)
	}
	if unit.Dir != "" {
		return stageUnit(d.path, unit, build)
	}

	return build(d.path)
}

func ownedBy(
	to string,
	source string,
) (bool, error) {
	info, err := os.Lstat(to)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("portable: stat %s: %w", to, err)
	}
	if !info.IsDir() {
		return false, nil
	}

	owner, err := os.ReadFile(filepath.Join(to, ownerMarker)) // #nosec G304 -- marker file inside the step's own destination directory

	return err == nil && strings.TrimSpace(string(owner)) == source, nil
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
