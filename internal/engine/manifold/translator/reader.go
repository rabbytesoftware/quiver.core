package translator

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/arrow"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/quiver"
)

// Module is the result of a successful Arrow() call.
type Module struct {
	Manifest    *domain.Arrow
	Precompiled map[string]models.PrecompiledTarget
	Selector    models.Selector
}

// QuiverModule is the raw translator output — manifest metadata plus unresolved arrow entries.
// Namespace derivation (path-based entries → full namespace) happens at the manifold layer.
type QuiverModule struct {
	Manifest domain.QuiverManifest
	Entries  []domain.QuiverArrowEntry
}

type Translator interface {
	Arrow(
		data []byte,
	) (Module, error)

	Quiver(
		data []byte,
	) (QuiverModule, error)

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
		arrowRegistry:  arrow.New(),
		quiverRegistry: quiver.NewRegistry(),
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

func (r *translator) Quiver(
	data []byte,
) (QuiverModule, error) {
	if yaml, ok := extractQuiverCodeblock(data); ok {
		data = yaml
	}
	return readQuiverManifest(
		data,
		func(v string) (quiver.Module, error) {
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

func readQuiverManifest(
	data []byte,
	getModule func(string) (quiver.Module, error),
) (QuiverModule, error) {
	info, err := extractManifestFromYAML(data)
	if err != nil {
		return QuiverModule{}, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if info.SchemaType != "quiver" {
		return QuiverModule{}, fmt.Errorf("schema type mismatch: expected quiver, got %s", info.SchemaType)
	}

	m, err := getModule(info.Version)
	if err != nil {
		return QuiverModule{}, fmt.Errorf("unsupported manifest %s: %w", info.ManifestKey, err)
	}

	schemaJSON, err := m.GetSchema()
	if err != nil {
		return QuiverModule{}, fmt.Errorf("failed to load schema for %s: %w", info.ManifestKey, err)
	}

	if err := validateYAML(schemaJSON, data); err != nil {
		return QuiverModule{}, fmt.Errorf("validation failed for %s: %w", info.ManifestKey, err)
	}

	manifest, entries, err := m.Map(data)
	if err != nil {
		return QuiverModule{}, fmt.Errorf("failed to map %s manifest: %w", info.ManifestKey, err)
	}

	return QuiverModule{
		Manifest: *manifest,
		Entries:  entries,
	}, nil
}
