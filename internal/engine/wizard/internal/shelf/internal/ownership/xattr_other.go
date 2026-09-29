//go:build !darwin

package ownership

import (
	"errors"
)

var errBundleTagUnsupported = errors.New("bundle ownership tags are only supported on darwin")

type unsupportedTagger struct{}

func NewTagger() Tagger {
	return &unsupportedTagger{}
}

func (*unsupportedTagger) Read(
	_ string,
) (string, error) {
	return "", errBundleTagUnsupported
}

func (*unsupportedTagger) Write(
	_ string,
	_ string,
) error {
	return errBundleTagUnsupported
}
