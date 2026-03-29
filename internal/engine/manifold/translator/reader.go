package translator

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/schemas"
)

var defaultTranslator = &Translator{registry: schemas.NewRegistry()}

// Arrow validates and parses arrow manifest YAML into a RawArrow using the default registry.
func Arrow(
	data []byte,
) (*models.RawArrow, error) {
	return defaultTranslator.Arrow(data)
}

// Quiver validates and parses quiver manifest YAML into a RawQuiver using the default registry.
func Quiver(
	data []byte,
) (*models.RawQuiver, error) {
	return defaultTranslator.Quiver(data)
}

// Translator validates and parses manifest YAML bytes into raw model types.
type Translator struct {
	registry *schemas.Registry
}

// NewTranslator returns a Translator backed by the default schema registry.
func NewTranslator() *Translator {
	return &Translator{
		registry: schemas.NewRegistry(),
	}
}

// Arrow validates and parses arrow manifest YAML bytes into a RawArrow.
func (r *Translator) Arrow(
	data []byte,
) (*models.RawArrow, error) {
	return readManifest(data, "arrow", r.registry.GetArrowMapper)
}

// Quiver validates and parses quiver manifest YAML bytes into a RawQuiver.
func (r *Translator) Quiver(
	data []byte,
) (*models.RawQuiver, error) {
	return readManifest(data, "quiver", r.registry.GetQuiverMapper)
}

// ReadSchemaInfo extracts the schema type and version from manifest YAML bytes.
func (r *Translator) ReadSchemaInfo(
	data []byte,
) (*ManifestInfo, error) {
	manifest, err := extractManifestFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest info: %w", err)
	}

	return manifest, nil
}

func readManifest[T any](
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

	mapper, err := getMapper(manifest.ManifestKey)
	if err != nil {
		return nil, fmt.Errorf("unsupported manifest %s: %w", manifest.ManifestKey, err)
	}

	schemaJSON, err := mapper.GetSchema()
	if err != nil {
		return nil, fmt.Errorf("failed to load schema for %s: %w", manifest.ManifestKey, err)
	}

	if err := validateYAML(schemaJSON, data); err != nil {
		return nil, fmt.Errorf("validation failed for %s: %w", manifest.ManifestKey, err)
	}

	result, err := mapper.Map(data)
	if err != nil {
		return nil, fmt.Errorf("failed to map %s manifest: %w", manifest.ManifestKey, err)
	}

	return result, nil
}
