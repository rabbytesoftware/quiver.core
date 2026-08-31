package translator

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/collection"
)

// Module is the result of a successful Arrow() call.
type Module struct {
	Manifest    *domain.Arrow
	Precompiled map[string]models.PrecompiledTarget
	Selector    models.Selector
}

// CollectionModule is the result of a successful Collection() call.
// Defined here (not in translator/collection/) to avoid an import cycle between
// the translator package and its sub-packages.
type CollectionModule struct {
	Manifest domain.Collection
	Entries  []domain.CollectionArrowEntry
}

type Translator interface {
	Arrow(
		data []byte,
	) (Module, error)

	Collection(
		data []byte,
	) (CollectionModule, error)

	ReadSchemaInfo(
		data []byte,
	) (*ManifestInfo, error)

	// ExtractReadme returns the prose surrounding an ARROW.md's fenced ```arrow
	// block, trimmed. ok is false when the input isn't the markdown manifest
	// form, or carries no prose alongside the block.
	ExtractReadme(
		data []byte,
	) (string, bool)
}

type translator struct {
	arrowRegistry      *arrow.Registry
	collectionRegistry *collection.Registry
}

func NewTranslator() Translator {
	return &translator{
		arrowRegistry:      arrow.New(),
		collectionRegistry: collection.NewRegistry(),
	}
}

func (r *translator) Arrow(
	data []byte,
) (Module, error) {
	if yaml, ok := extractArrowCodeblock(data); ok {
		data = yaml
	}

	manifest, err := extractManifestFromYAML(data)
	if err != nil {
		return Module{}, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if manifest.SchemaType != "arrow" {
		return Module{}, fmt.Errorf("schema type mismatch: expected arrow, got %s", manifest.SchemaType)
	}

	m, err := r.arrowRegistry.Get(manifest.Version)
	if err != nil {
		return Module{}, fmt.Errorf("unsupported manifest %s: %w", manifest.ManifestKey, err)
	}

	schemaJSON := m.Schema()

	if err := validateYAML(schemaJSON, data); err != nil {
		return Module{}, fmt.Errorf("validation failed for %s: %w", manifest.ManifestKey, err)
	}

	parsed, precompiled, err := m.Parse(data)
	if err != nil {
		return Module{}, err
	}

	return Module{
		Manifest:    parsed,
		Precompiled: precompiled,
		Selector:    m.Selector(),
	}, nil
}

func (r *translator) Collection(
	data []byte,
) (CollectionModule, error) {
	if yaml, ok := extractCollectionCodeblock(data); ok {
		data = yaml
	}
	return readCollectionManifest(
		data,
		func(v string) (collection.Module, error) {
			return r.collectionRegistry.Get(v)
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

func (r *translator) ExtractReadme(
	data []byte,
) (string, bool) {
	return extractArrowReadme(data)
}

func readCollectionManifest(
	data []byte,
	getModule func(string) (collection.Module, error),
) (CollectionModule, error) {
	info, err := extractManifestFromYAML(data)
	if err != nil {
		return CollectionModule{}, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if info.SchemaType != "collection" {
		return CollectionModule{}, fmt.Errorf("schema type mismatch: expected collection, got %s", info.SchemaType)
	}

	m, err := getModule(info.Version)
	if err != nil {
		return CollectionModule{}, fmt.Errorf("unsupported manifest %s: %w", info.ManifestKey, err)
	}

	schemaJSON, err := m.GetSchema()
	if err != nil {
		return CollectionModule{}, fmt.Errorf("failed to load schema for %s: %w", info.ManifestKey, err)
	}

	if err := validateYAML(schemaJSON, data); err != nil {
		return CollectionModule{}, fmt.Errorf("validation failed for %s: %w", info.ManifestKey, err)
	}

	manifest, entries, err := m.Map(data)
	if err != nil {
		return CollectionModule{}, fmt.Errorf("failed to map %s manifest: %w", info.ManifestKey, err)
	}

	return CollectionModule{
		Manifest: *manifest,
		Entries:  entries,
	}, nil
}
