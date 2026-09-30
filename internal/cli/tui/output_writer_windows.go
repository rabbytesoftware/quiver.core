//go:build windows

package tui

import (
	"errors"
	"syscall"
)

// errNoData is ERROR_NO_DATA, what Windows reports writing to a pipe whose
// reader closed it; package syscall does not name it.
const errNoData = syscall.Errno(232)

func isPlatformBrokenPipe(err error) bool {
	return errors.Is(err, syscall.ERROR_BROKEN_PIPE) || errors.Is(err, errNoData)
}
