package portable

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
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
