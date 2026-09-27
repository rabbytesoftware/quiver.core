//go:build !windows

package shelf

import (
	"errors"
)

var errUserPathUnsupported = errors.New("user path registry is only available on windows")

type unsupportedUserPath struct{}

var defaultUserPath userPath = &unsupportedUserPath{}

func (*unsupportedUserPath) read() (string, error) {
	return "", errUserPathUnsupported
}

func (*unsupportedUserPath) write(
	_ string,
) error {
	return errUserPathUnsupported
}
