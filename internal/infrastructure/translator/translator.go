package translator

import (
	"context"

	fns "github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator/parser"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator/reader"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/quiver"
)

type Translator interface {
	TranslateArrow(
		ctx context.Context,
		manifestPath string,
	) (*arrow.Arrow, error)

	TranslateQuiver(
		ctx context.Context,
		manifestPath string,
	) (*quiver.Quiver, error)

	GetManifestInfo(
		ctx context.Context,
		manifestPath string,
	) (*parser.ManifestInfo, error)

	IsCompatible(
		ctx context.Context,
		manifestPath, schemaType string,
	) (bool, error)
}

type TranslatorImplementation struct {
	fns    fns.FNSInterface
	reader reader.Reader
}

func NewTranslator(fns fns.FNSInterface) Translator {
	return &TranslatorImplementation{
		fns:    fns,
		reader: reader.NewReader(),
	}
}

func (t *TranslatorImplementation) TranslateArrow(
	ctx context.Context,
	manifestPath string,
) (*arrow.Arrow, error) {
	return t.reader.ReadArrow(manifestPath)
}

func (t *TranslatorImplementation) TranslateQuiver(
	ctx context.Context,
	manifestPath string,
) (*quiver.Quiver, error) {
	return t.reader.ReadQuiver(manifestPath)
}

func (t *TranslatorImplementation) GetManifestInfo(
	ctx context.Context,
	manifestPath string,
) (*parser.ManifestInfo, error) {
	return t.reader.ReadManifestInfo(manifestPath)
}

func (t *TranslatorImplementation) IsCompatible(
	ctx context.Context,
	manifestPath, schemaType string,
) (bool, error) {
	info, err := t.reader.ReadManifestInfo(manifestPath)
	if err != nil {
		return false, err
	}
	return info.SchemaType == schemaType, nil
}
