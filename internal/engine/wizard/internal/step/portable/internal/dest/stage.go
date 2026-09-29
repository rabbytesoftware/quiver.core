package dest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

const stagingSuffix = ".quiver-tmp"

func stageOwned(
	to string,
	source string,
	unit unpack.Unit,
	build Build,
) (unpack.Result, error) {
	staging := sibling(to, stagingSuffix)
	aside := sibling(to, workfs.AsideSuffix)
	for _, leftover := range []string{staging, aside} {
		if err := os.RemoveAll(leftover); err != nil {
			return unpack.Result{}, fmt.Errorf("portable: remove leftover %s: %w", leftover, err)
		}
	}

	result, err := build(filepath.Join(staging, unit.Dir))
	if err == nil {
		err = markOwned(staging, source)
	}
	if err == nil {
		err = workfs.Swap(staging, to, aside, os.Rename)
	}
	if err != nil {
		_ = os.RemoveAll(staging)
		return unpack.Result{}, fmt.Errorf("portable: %w", err)
	}
	_ = os.RemoveAll(aside)

	return relocate(result, staging, to), nil
}

func stageUnit(
	to string,
	unit unpack.Unit,
	build Build,
) (unpack.Result, error) {
	final, staging, err := unitPaths(to, unit.Dir)
	if err != nil {
		return unpack.Result{}, err
	}

	if err := os.RemoveAll(staging); err != nil {
		return unpack.Result{}, fmt.Errorf("portable: remove leftover %s: %w", staging, err)
	}

	result, err := build(staging)
	if err == nil {
		err = replaceUnit(staging, final, unit.Proof)
	}
	if err != nil {
		_ = os.RemoveAll(staging)
		return unpack.Result{}, err
	}

	return relocate(result, staging, final), nil
}

func unitPaths(
	to string,
	name string,
) (string, string, error) {
	parent := filepath.Clean(to)
	final := filepath.Join(to, name)
	staging := sibling(final, stagingSuffix)
	if name == "." || name == ".." || filepath.Dir(final) != parent || filepath.Dir(staging) != parent {
		return "", "", fmt.Errorf("portable: %q is not a direct child of %s", name, to)
	}

	return final, staging, nil
}

func replaceUnit(
	staging string,
	final string,
	proof string,
) error {
	if err := removeUnit(final, proof); err != nil {
		return err
	}

	if err := os.Rename(staging, final); err != nil {
		return fmt.Errorf("portable: move %s into place: %w", final, err)
	}

	return nil
}

func removeUnit(
	final string,
	proof string,
) error {
	_, err := os.Lstat(final)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("portable: stat %s: %w", final, err)
	}

	if _, err := os.Lstat(filepath.Join(final, proof)); err != nil {
		return fmt.Errorf("%w: %s has no %s", ErrUnowned, final, proof)
	}

	if err := os.RemoveAll(final); err != nil {
		return fmt.Errorf("portable: remove previous %s: %w", final, err)
	}

	return nil
}

func relocate(
	result unpack.Result,
	from string,
	to string,
) unpack.Result {
	apps := make([]unpack.App, 0, len(result.Apps))
	for _, app := range result.Apps {
		apps = append(apps, unpack.App{
			Name:  app.Name,
			Entry: workfs.Relocate(app.Entry, from, to),
			Icon:  workfs.Relocate(app.Icon, from, to),
		})
	}

	return unpack.Result{Apps: apps, Output: workfs.Relocate(result.Output, from, to)}
}

func sibling(
	path string,
	suffix string,
) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+suffix)
}
