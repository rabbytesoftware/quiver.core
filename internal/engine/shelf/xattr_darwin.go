//go:build darwin

package shelf

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

const bundleOwnerAttr = "net.quiver.namespace"

type xattrTagger struct{}

var defaultTagger bundleTagger = &xattrTagger{}

func (*xattrTagger) read(
	path string,
) (string, error) {
	size, err := unix.Lgetxattr(path, bundleOwnerAttr, nil)
	if errors.Is(err, unix.ENOATTR) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("getxattr %s: %w", path, err)
	}

	buf := make([]byte, size)
	n, err := unix.Lgetxattr(path, bundleOwnerAttr, buf)
	if err != nil {
		return "", fmt.Errorf("getxattr %s: %w", path, err)
	}
	return string(buf[:n]), nil
}

func (*xattrTagger) write(
	path string,
	value string,
) error {
	if err := unix.Lsetxattr(path, bundleOwnerAttr, []byte(value), 0); err != nil {
		return fmt.Errorf("setxattr %s: %w", path, err)
	}
	return nil
}
