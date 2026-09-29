package install

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
)

const executablePerm = 0o755

func (i *installer) installExecutable(
	ctx context.Context,
	src *os.File,
	info os.FileInfo,
	from string,
	to string,
	name string,
) (string, error) {
	if info.Size() > i.maxBytes {
		return "", sizeError{path: from, size: info.Size(), limit: i.maxBytes}
	}

	if err := fns.MkdirAll(ctx, to, executablePerm); err != nil {
		return "", fmt.Errorf("portable: create %s: %w", to, err)
	}

	out := filepath.Join(to, cmp.Or(name, filepath.Base(from)))
	if existing, err := os.Stat(out); err == nil && os.SameFile(info, existing) {
		return from, chmodExecutable(out)
	}

	if err := copyExecutable(src, info.Size(), out); err != nil {
		return "", fmt.Errorf("portable: copy %s: %w", out, err)
	}

	return out, nil
}

func copyExecutable(
	src io.ReaderAt,
	size int64,
	out string,
) error {
	_ = os.Remove(out)

	dst, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- path is the step's own output, resolved against the workdir
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, io.NewSectionReader(src, 0, size))
	if err := errors.Join(err, dst.Close()); err != nil {
		_ = os.Remove(out)
		return err
	}

	return chmodExecutable(out)
}

func chmodExecutable(
	path string,
) error {
	if err := os.Chmod(path, executablePerm); err != nil {
		return fmt.Errorf("portable: chmod %s: %w", path, err)
	}

	return nil
}
