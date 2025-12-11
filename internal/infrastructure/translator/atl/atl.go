package atl

import (
	"context"

	translator "github.com/rabbytesoftware/quiver/internal/infrastructure/translator/models"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
)

type ArrowTranslationLayer struct {
}

func NewATL() translator.TranslatorLayerInterface[arrow.Arrow] {
	return &ArrowTranslationLayer{}
}

func (a *ArrowTranslationLayer) IsCompatible(
	ctx context.Context,
	manifestPath string,
) (bool, error) {
	return false, nil
}

func (a *ArrowTranslationLayer) Translate(
	ctx context.Context,
	manifestPath string,
) (*arrow.Arrow, error) {
	return nil, nil
}

func (a *ArrowTranslationLayer) GetManifestVersion(
	ctx context.Context,
	manifestPath string,
) (string, error) {
	return "", nil
}

func (a *ArrowTranslationLayer) GetSupportedVersions(
	ctx context.Context,
) ([]string, error) {
	return nil, nil
}
