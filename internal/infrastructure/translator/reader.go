package translator

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/fns"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator/schemas"
	"github.com/rabbytesoftware/quiver/internal/domain"
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

func (r *Translator) Arrow(
	manifestPath string,
) (*domain.Arrow, error) {
	return readManifest(r, manifestPath, "arrow", r.registry.GetArrowMapper)
}

func (r *Translator) Quiver(
	manifestPath string,
) (*domain.Quiver, error) {
	return readManifest(r, manifestPath, "quiver", r.registry.GetQuiverMapper)
}

func (r *Translator) ReadSchemaInfo(
	manifestPath string,
) (*ManifestInfo, error) {
	yamlData, err := fns.Read(context.Background(), manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest info from %s: %w", manifestPath, err)
	}

	manifest, err := extractManifestFromYAML(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest info from %s: %w", manifestPath, err)
	}

	return manifest, nil
}

func readManifest[T any](
	r *Translator,
	manifestPath string,
	expectedSchemaType string,
	getMapper func(string) (schemas.Mapper[T], error),
) (*T, error) {
	yamlData, err := fns.Read(context.Background(), manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file %s: %w", manifestPath, err)
	}

	manifest, err := extractManifestFromYAML(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest from %s: %w", manifestPath, err)
	}

	if manifest.SchemaType != expectedSchemaType {
		return nil, fmt.Errorf("schema type mismatch in %s: expected %s, got %s",
			manifestPath, expectedSchemaType, manifest.SchemaType)
	}

	if !r.registry.IsSupported(manifest.ManifestKey) {
		return nil, fmt.Errorf("unsupported manifest %s in %s",
			manifest.ManifestKey, manifestPath)
	}

	schemaJSON, err := r.registry.GetSchema(manifest.ManifestKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema for %s from %s: %w",
			manifest.ManifestKey, manifestPath, err)
	}

	if err := validateYAML(schemaJSON, yamlData); err != nil {
		return nil, fmt.Errorf("validation failed for %s in %s: %w",
			manifest.ManifestKey, manifestPath, err)
	}

	mapper, err := getMapper(manifest.ManifestKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get mapper for %s from %s: %w",
			manifest.ManifestKey, manifestPath, err)
	}

	var dataMap map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &dataMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML from %s: %w", manifestPath, err)
	}

	result, err := mapper.Map(dataMap)
	if err != nil {
		return nil, fmt.Errorf("failed to map %s manifest from %s to domain model: %w",
			manifest.ManifestKey, manifestPath, err)
	}

	return result, nil
}
