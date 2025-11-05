package reader

import (
	"fmt"
	"os"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator/parser"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator/schemas"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator/validator"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/quiver"
	"gopkg.in/yaml.v3"
)

type ReaderImplementation struct {
	parser    parser.Parser
	validator validator.Validator
	registry  schemas.Registry
}

func NewReader() Reader {
	return &ReaderImplementation{
		parser:    parser.NewParser(),
		validator: validator.NewValidator(),
		registry:  schemas.NewRegistry(),
	}
}

func (r *ReaderImplementation) ReadArrow(manifestPath string) (*arrow.Arrow, error) {
	return readManifest(r, manifestPath, "arrow", r.registry.GetArrowMapper)
}

func (r *ReaderImplementation) ReadQuiver(manifestPath string) (*quiver.Quiver, error) {
	return readManifest(r, manifestPath, "quiver", r.registry.GetQuiverMapper)
}

func readManifest[T any](
	r *ReaderImplementation,
	manifestPath string,
	expectedSchemaType string,
	getMapper func(string) (schemas.Mapper[T], error),
) (*T, error) {
	// Step 1: Read file
	yamlData, err := r.readFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file %s: %w", manifestPath, err)
	}

	// Step 2: Parse manifest metadata
	manifest, err := r.parser.ParseFromYAML(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest from %s: %w", manifestPath, err)
	}

	// Step 3: Verify schema type
	if manifest.SchemaType != expectedSchemaType {
		return nil, fmt.Errorf("schema type mismatch in %s: expected %s, got %s",
			manifestPath, expectedSchemaType, manifest.SchemaType)
	}

	// Step 4: Check if version is supported
	if !r.registry.IsSupported(manifest.ManifestKey) {
		return nil, fmt.Errorf("unsupported manifest %s in %s",
			manifest.ManifestKey, manifestPath)
	}

	// Step 5: Get JSON schema for validation
	schemaJSON, err := r.registry.GetSchema(manifest.ManifestKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema for %s from %s: %w",
			manifest.ManifestKey, manifestPath, err)
	}

	// Step 6: Validate against schema
	validationResult, err := r.validator.Validate(schemaJSON, yamlData)
	if err != nil {
		return nil, fmt.Errorf("validation error for %s in %s: %w",
			manifest.ManifestKey, manifestPath, err)
	}

	if !validationResult.Valid {
		return nil, fmt.Errorf("validation failed for %s manifest %s in %s: %v",
			expectedSchemaType, manifest.ManifestKey, manifestPath, validationResult.Errors)
	}

	// Step 7: Get version-specific mapper
	mapper, err := getMapper(manifest.ManifestKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get mapper for %s from %s: %w",
			manifest.ManifestKey, manifestPath, err)
	}

	// Step 8: Unmarshal to map for mapper
	var dataMap map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &dataMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML from %s: %w", manifestPath, err)
	}

	// Step 9: Map to domain model
	result, err := mapper.Map(dataMap)
	if err != nil {
		return nil, fmt.Errorf("failed to map %s manifest from %s to domain model: %w",
			manifest.ManifestKey, manifestPath, err)
	}

	return result, nil
}

func (r *ReaderImplementation) ReadManifestInfo(manifestPath string) (*parser.ManifestInfo, error) {
	yamlData, err := r.readFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest info from %s: %w", manifestPath, err)
	}

	manifest, err := r.parser.ParseFromYAML(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest info from %s: %w", manifestPath, err)
	}

	return manifest, nil
}

func (r *ReaderImplementation) readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return data, nil
}
