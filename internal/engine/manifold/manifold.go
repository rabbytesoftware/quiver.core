package manifold

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator"
)

// Manifold resolves arrow and quiver manifests from remote git repositories.
// Given a namespace it fetches the YAML manifest, validates and parses it,
// then assembles the result into a domain aggregate.
type Manifold interface {
	// ResolveArrow fetches and assembles an ArrowManifest for the given namespace and OS.
	ResolveArrow(
		ctx context.Context,
		namespace domain.Namespace,
		os string,
	) (*domain.ArrowManifest, error)

	// ResolveQuiver fetches and assembles a QuiverManifest for the given namespace.
	ResolveQuiver(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.QuiverManifest, error)
}

type manifestResolver interface {
	ResolveArrow(ctx context.Context, namespace domain.Namespace) ([]byte, error)
	ResolveQuiver(ctx context.Context, namespace domain.Namespace) ([]byte, error)
}

type arrowParser func(data []byte) (*models.RawArrow, error)
type quiverParser func(data []byte) (*models.RawQuiver, error)

// New returns a Manifold. If fetchTimeout is zero, a 30s default is used.
func New(
	fetchTimeout time.Duration,
) Manifold {
	return &manifold{
		resolver:    resolver.New(fetchTimeout),
		assembler:   assembler.New(),
		parseArrow:  translator.Arrow,
		parseQuiver: translator.Quiver,
	}
}

type manifold struct {
	resolver    manifestResolver
	assembler   *assembler.Assembler
	parseArrow  arrowParser
	parseQuiver quiverParser
}

func (m *manifold) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
	os string,
) (*domain.ArrowManifest, error) {
	data, err := m.resolver.ResolveArrow(ctx, namespace)
	if err != nil {
		return nil, err
	}

	raw, err := m.parseArrow(data)
	if err != nil {
		return nil, err
	}

	return m.assembler.AssembleArrow(raw, os)
}

func (m *manifold) ResolveQuiver(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.QuiverManifest, error) {
	data, err := m.resolver.ResolveQuiver(ctx, namespace)
	if err != nil {
		return nil, err
	}

	raw, err := m.parseQuiver(data)
	if err != nil {
		return nil, err
	}

	return m.assembler.AssembleQuiver(raw)
}
