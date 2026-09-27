//go:build windows

package shelf

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

type registryUserPath struct{}

var defaultUserPath userPath = &registryUserPath{}

func (*registryUserPath) read() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open user environment: %w", err)
	}
	defer func() { _ = k.Close() }()

	value, _, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read user path: %w", err)
	}
	return value, nil
}

func (*registryUserPath) write(
	value string,
) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open user environment: %w", err)
	}
	defer func() { _ = k.Close() }()

	if err := k.SetExpandStringValue("Path", value); err != nil {
		return fmt.Errorf("write user path: %w", err)
	}
	return nil
}
