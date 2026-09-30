//go:build !windows

package userpath

import (
	"errors"
)

const registryLocation = `HKCU\Environment\Path`

var errUnsupported = errors.New("user path registry is only available on windows")

type unsupported struct{}

func New() UserPath {
	return &unsupported{}
}

func (*unsupported) Read() (string, error) {
	return "", errUnsupported
}

func (*unsupported) Write(
	_ string,
) error {
	return errUnsupported
}

func (*unsupported) Broadcast() error {
	return errUnsupported
}

func (*unsupported) Location() string {
	return registryLocation
}
