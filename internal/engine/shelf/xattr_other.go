//go:build !darwin

package shelf

import (
	"errors"
)

var errBundleTagUnsupported = errors.New("bundle ownership tags are only supported on darwin")

type unsupportedTagger struct{}

var defaultTagger bundleTagger = &unsupportedTagger{}

func (*unsupportedTagger) read(
	_ string,
) (string, error) {
	return "", errBundleTagUnsupported
}

func (*unsupportedTagger) write(
	_ string,
	_ string,
) error {
	return errBundleTagUnsupported
}
