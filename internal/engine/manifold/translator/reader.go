package translator

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/schemas"
	"gopkg.in/yaml.v3"
)

type Translator struct {
	registry *schemas.Registry
}

func NewTranslator() *Translator {
	return &Translator{
		registry: schemas.NewRegistry(),
	}
}

func (r *Translator) Arrow(data []byte) (*domain.Arrow, error) {
	return readManifest(r, data, "arrow", r.registry.GetArrowMapper)
}

func (r *Translator) Quiver(data []byte) (*domain.Quiver, error) {
	return readManifest(r, data, "quiver", r.registry.GetQuiverMapper)
}

func (r *Translator) ReadSchemaInfo(data []byte) (*ManifestInfo, error) {
	manifest, err := extractManifestFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest info: %w", err)
	}

	return manifest, nil
}

func readManifest[T any](
	r *Translator,
	data []byte,
	expectedSchemaType string,
	getMapper func(string) (schemas.Mapper[T], error),
) (*T, error) {
	manifest, err := extractManifestFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if manifest.SchemaType != expectedSchemaType {
		return nil, fmt.Errorf("schema type mismatch: expected %s, got %s",
			expectedSchemaType, manifest.SchemaType)
	}

	if !r.registry.IsSupported(manifest.ManifestKey) {
		return nil, fmt.Errorf("unsupported manifest %s", manifest.ManifestKey)
	}

	schemaJSON, err := r.registry.GetSchema(manifest.ManifestKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema for %s: %w", manifest.ManifestKey, err)
	}

	if err := validateYAML(schemaJSON, data); err != nil {
		return nil, fmt.Errorf("validation failed for %s: %w", manifest.ManifestKey, err)
	}

	mapper, err := getMapper(manifest.ManifestKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get mapper for %s: %w", manifest.ManifestKey, err)
	}

	var dataMap map[string]interface{}
	if err := yaml.Unmarshal(data, &dataMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	result, err := mapper.Map(dataMap)
	if err != nil {
		return nil, fmt.Errorf("failed to map %s manifest to domain model: %w", manifest.ManifestKey, err)
	}

	return result, nil
}
