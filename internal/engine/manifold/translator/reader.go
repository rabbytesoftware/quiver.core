package translator

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/arrow"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/quiver"
)

type Translator interface {
	Arrow(
		data []byte,
	) (*domain.ArrowManifest, error)

	Quiver(
		data []byte,
	) (*domain.QuiverManifest, error)

	ReadSchemaInfo(
		data []byte,
	) (*ManifestInfo, error)
}

type translator struct {
	arrowRegistry  *arrow.Registry
	quiverRegistry *quiver.Registry
}

func NewTranslator() Translator {
	return &translator{
		arrowRegistry:  arrow.NewRegistry(),
		quiverRegistry: quiver.NewRegistry(),
	}
}

func (r *translator) Arrow(
	data []byte,
) (*domain.ArrowManifest, error) {
	return readManifest(
		data,
		"arrow",
		func(v string) (module[domain.ArrowManifest], error) {
			return r.arrowRegistry.Get(v)
		},
	)
}

func (r *translator) Quiver(
	data []byte,
) (*domain.QuiverManifest, error) {
	return readManifest(
		data,
		"quiver",
		func(v string) (module[domain.QuiverManifest], error) {
			return r.quiverRegistry.Get(v)
		},
	)
}

func (r *translator) ReadSchemaInfo(
	data []byte,
) (*ManifestInfo, error) {
	manifest, err := extractManifestFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest info: %w", err)
	}

	return manifest, nil
}

type module[T any] interface {
	GetSchema() ([]byte, error)
	Map(data []byte) (*T, error)
}

func readManifest[T any](
	data []byte,
	expectedSchemaType string,
	getModule func(string) (module[T], error),
) (*T, error) {
	manifest, err := extractManifestFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if manifest.SchemaType != expectedSchemaType {
		return nil, fmt.Errorf("schema type mismatch: expected %s, got %s",
			expectedSchemaType, manifest.SchemaType)
	}

	m, err := getModule(manifest.Version)
	if err != nil {
		return nil, fmt.Errorf("unsupported manifest %s: %w", manifest.ManifestKey, err)
	}

	schemaJSON, err := m.GetSchema()
	if err != nil {
		return nil, fmt.Errorf("failed to load schema for %s: %w", manifest.ManifestKey, err)
	}

	if err := validateYAML(schemaJSON, data); err != nil {
		return nil, fmt.Errorf("validation failed for %s: %w", manifest.ManifestKey, err)
	}

	result, err := m.Map(data)
	if err != nil {
		return nil, fmt.Errorf("failed to map %s manifest: %w", manifest.ManifestKey, err)
	}

	return result, nil
}
