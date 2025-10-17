package translator

import (
	"errors"

	"github.com/hashicorp/go-version"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
)

type ArrowManifest struct {
	Name         string
	Version      string
	MinSupported string
	MaxSupported string
	Capabilities []string
	Dependencies []string
}

type ArrowTranslator interface {
	ValidateArrow(currentVersion string, targetVersion string, a arrow.Arrow) error
	TranslateArrow(fromVersion string, toVersion string, a arrow.Arrow) (ArrowManifest, error)
	ParseManifest(data []byte) (ArrowManifest, error)
}

type ArrowTranslatorImpl struct{}

func (t *ArrowTranslatorImpl) ValidateArrow(currentVersion string, targetVersion string, a arrow.Arrow) error {
	currVer, err := version.NewVersion(currentVersion)
	if err != nil {
		return errors.New("invalid current version format")
	}

	pkgVer, err := version.NewVersion(a.Version)
	if err != nil {
		return errors.New("invalid package version format")
	}

	if currVer.LessThan(pkgVer) {
		return errors.New("current version is older than package version")
	}

	return nil
}
