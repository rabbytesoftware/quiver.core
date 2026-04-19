package manifold

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver"
	resolvers "github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver/resolvers"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator"
)

// Manifold resolves arrow and quiver manifests from remote git repositories.
// Given a namespace it fetches the YAML manifest, validates and parses it,
// then validates the result with business rules.
type Manifold interface {
	// ResolveArrow fetches and validates an ArrowManifest for the given namespace.
	// The returned aggregate includes compiled OS-specific targets in manifest.Targets.
	ResolveArrow(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.Arrow, error)

	// ResolveQuiver fetches and validates a QuiverManifest for the given namespace.
	ResolveQuiver(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.QuiverManifest, error)

	// ParseArrow translates and validates a raw YAML arrow manifest without
	// fetching from a remote source. Returns RuleErrors if validation fails.
	ParseArrow(
		data []byte,
	) (*domain.Arrow, error)

	// ResolveConstraint resolves a glob constraint pattern to a concrete ref
	// (e.g. tag) for the given namespace.
	ResolveConstraint(
		ctx context.Context,
		ns domain.Namespace,
		pattern string,
	) (string, error)
}

type manifold struct {
	rsv        resolver.Resolver
	trs        translator.Translator
	cmp        compiler.Compiler
	rls        ruleset.Ruleset
	constraint resolvers.ConstraintResolver
}

func New(
	fetchTimeout time.Duration,
) Manifold {
	return &manifold{
		rsv:        resolver.New(fetchTimeout),
		trs:        translator.NewTranslator(),
		cmp:        compiler.New(),
		rls:        ruleset.New(),
		constraint: resolvers.NewConstraintResolver(fetchTimeout),
	}
}

func (m *manifold) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.Arrow, error) {
	data, err := m.rsv.ResolveArrow(ctx, namespace)
	if err != nil {
		return nil, err
	}

	return m.ParseArrow(data)
}

func (m *manifold) ParseArrow(
	data []byte,
) (*domain.Arrow, error) {
	module, err := m.trs.Arrow(data)
	if err != nil {
		return nil, err
	}

	if err := m.rls.ValidatePrecompile(module.Manifest, module.Precompiled); err != nil {
		return nil, err
	}

	if err := m.cmp.Compile(module.Manifest, module.Precompiled, module.Selector); err != nil {
		return nil, err
	}

	if err := m.rls.ValidateCompiled(module.Manifest); err != nil {
		return nil, err
	}

	return module.Manifest, nil
}

func (m *manifold) ResolveConstraint(
	ctx context.Context,
	ns domain.Namespace,
	pattern string,
) (string, error) {
	return m.constraint.Resolve(ctx, ns, pattern)
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

	if err := ruleset.ValidateQuiver(manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}
