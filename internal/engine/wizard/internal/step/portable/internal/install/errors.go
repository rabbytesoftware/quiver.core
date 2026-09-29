package install

import (
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
)

var (
	ErrInvalidName   = errors.New("portable: name must be a single file name")
	ErrUnknownFormat = errors.New("portable: unknown format")
	ErrUnownedAppDir = errors.New("portable: appdir not managed by quiver")
)

type sizeError struct {
	path  string
	size  int64
	limit int64
}

func (e sizeError) Error() string {
	return fmt.Sprintf("portable: executable %s is %d bytes, exceeds the %d-byte limit", e.path, e.size, e.limit)
}

func (e sizeError) Unwrap() error {
	return unpack.ErrTooLarge
}
