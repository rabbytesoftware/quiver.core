//go:build !windows

package pathenv

import (
	"errors"
)

var errUserPathUnsupported = errors.New("user path registry is only available on windows")

type unsupportedUserPath struct{}

func NewUserPath() UserPath {
	return &unsupportedUserPath{}
}

func (*unsupportedUserPath) Read() (string, error) {
	return "", errUserPathUnsupported
}

func (*unsupportedUserPath) Write(
	_ string,
) error {
	return errUserPathUnsupported
}
