package manifold

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator"
)

// Manifold resolves arrow and quiver manifests from remote git repositories.
// Given a namespace it fetches the YAML manifest, validates and parses it,
// then validates the result with business rules.
type Manifold interface {
	// ResolveArrow fetches and validates an ArrowManifest for the given namespace.
	// The returned aggregate includes all OS/arch variants via OverrideableString fields.
	ResolveArrow(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.ArrowManifest, error)

	// ResolveQuiver fetches and validates a QuiverManifest for the given namespace.
	ResolveQuiver(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.QuiverManifest, error)

	// ParseArrow translates and validates a raw YAML arrow manifest without
	// fetching from a remote source. Returns AssemblerErrors if validation fails.
	ParseArrow(data []byte) (*domain.ArrowManifest, error)
}

type manifold struct {
	rsv resolver.Resolver
	trs translator.Translator
}

func New(
	fetchTimeout time.Duration,
) Manifold {
	return &manifold{
		rsv: resolver.New(fetchTimeout),
		trs: translator.NewTranslator(),
	}
}

func (m *manifold) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.ArrowManifest, error) {
	data, err := m.rsv.ResolveArrow(ctx, namespace)
	if err != nil {
		return nil, err
	}

	manifest, err := m.trs.Arrow(data)
	if err != nil {
		return nil, err
	}

	if err := assembler.ValidateArrow(manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

func (m *manifold) ParseArrow(data []byte) (*domain.ArrowManifest, error) {
	manifest, err := m.trs.Arrow(data)
	if err != nil {
		return nil, err
	}
	if err := assembler.ValidateArrow(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func (m *manifold) ResolveQuiver(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.QuiverManifest, error) {
	data, err := m.rsv.ResolveQuiver(ctx, namespace)
	if err != nil {
		return nil, err
	}

	manifest, err := m.trs.Quiver(data)
	if err != nil {
		return nil, err
	}

	if err := assembler.ValidateQuiver(manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}
