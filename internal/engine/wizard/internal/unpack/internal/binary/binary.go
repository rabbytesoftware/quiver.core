package binary

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

const (
	magicLen       = 4
	executablePerm = 0o755
)

type binary struct {
	src      *os.File
	maxBytes int64
}

type sizeError struct {
	path  string
	size  int64
	limit int64
}

func New(
	maxBytes int64,
) models.Detect {
	return func(src *os.File, _ int64) (models.Format, bool, error) {
		if !isExecutable(src) {
			return nil, false, nil
		}

		return &binary{src: src, maxBytes: maxBytes}, true, nil
	}
}

func (b *binary) Kind() models.Kind {
	return models.KindBinary
}

func (b *binary) Unit() models.Unit {
	return models.Unit{}
}

func (b *binary) Unpack(
	ctx context.Context,
	target models.Target,
) (models.Result, error) {
	from := b.src.Name()
	info, err := b.src.Stat()
	if err != nil {
		return models.Result{}, fmt.Errorf("unpack: stat %s: %w", from, err)
	}
	if info.Size() > b.maxBytes {
		return models.Result{}, sizeError{path: from, size: info.Size(), limit: b.maxBytes}
	}

	if err := fns.MkdirAll(ctx, target.Dir, executablePerm); err != nil {
		return models.Result{}, fmt.Errorf("unpack: create %s: %w", target.Dir, err)
	}

	out := filepath.Join(target.Dir, cmp.Or(target.Name, filepath.Base(from)))
	if existing, err := os.Stat(out); err == nil && os.SameFile(info, existing) {
		return models.Result{Output: from}, chmodExecutable(out)
	}

	if err := copyExecutable(b.src, info.Size(), out); err != nil {
		return models.Result{}, fmt.Errorf("unpack: copy %s: %w", out, err)
	}

	return models.Result{Output: out}, nil
}

func (e sizeError) Error() string {
	return fmt.Sprintf("unpack: executable %s is %d bytes, exceeds the %d-byte limit", e.path, e.size, e.limit)
}

func (e sizeError) Unwrap() error {
	return models.ErrTooLarge
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
		return fmt.Errorf("unpack: chmod %s: %w", path, err)
	}

	return nil
}

func isExecutable(
	src io.ReaderAt,
) bool {
	head := make([]byte, magicLen)
	n, _ := src.ReadAt(head, 0)
	head = head[:n]

	for _, magic := range []string{"\x7fELF", "\xfe\xed\xfa\xcf", "\xcf\xfa\xed\xfe", "\xca\xfe\xba\xbe", "MZ"} {
		if bytes.HasPrefix(head, []byte(magic)) {
			return true
		}
	}

	return false
}
