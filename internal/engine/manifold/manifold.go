package manifold

import (
	"context"
	"fmt"
	"strings"
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
	// Also returns the raw manifest bytes and the filename it was resolved from.
	ResolveArrow(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.Arrow, []byte, string, error)

	// ResolveCollection fetches and validates a Quiver for the given namespace.
	ResolveCollection(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.Collection, error)

	// ParseCollection translates and validates a raw quiver manifest (YAML or QUIVER.md bytes)
	// without fetching from a remote source. Derives local arrow namespaces from ns.
	ParseCollection(
		data []byte,
		ns domain.Namespace,
	) (*domain.Collection, error)

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

// NewWithResolvers builds a Manifold with injected resolver and constraint resolver.
// Intended for tests that need to control how namespaces are resolved.
func NewWithResolvers(
	rsv resolver.Resolver,
	crs resolvers.ConstraintResolver,
) Manifold {
	return &manifold{
		rsv:        rsv,
		trs:        translator.NewTranslator(),
		cmp:        compiler.New(),
		rls:        ruleset.New(),
		constraint: crs,
	}
}

func (m *manifold) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	raw, filename, err := m.rsv.ResolveArrow(ctx, namespace)
	if err != nil {
		return nil, nil, "", err
	}

	arrow, err := m.ParseArrow(raw)
	if err != nil {
		return nil, nil, "", err
	}

	return arrow, raw, filename, nil
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

func (m *manifold) ResolveCollection(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.Collection, error) {
	data, err := m.rsv.ResolveCollection(ctx, namespace)
	if err != nil {
		return nil, err
	}
	return m.ParseCollection(data, namespace)
}

func (m *manifold) ParseCollection(
	data []byte,
	ns domain.Namespace,
) (*domain.Collection, error) {
	mod, err := m.trs.Collection(data)
	if err != nil {
		return nil, err
	}

	if err := m.rls.ValidateCollectionEntries(mod.Entries); err != nil {
		return nil, err
	}

	arrows, err := deriveArrows(mod.Entries, ns)
	if err != nil {
		return nil, err
	}

	coll := mod.Manifest
	coll.Namespace = ns
	coll.Arrows = arrows

	if err := m.rls.ValidateCollection(&coll); err != nil {
		return nil, err
	}
	return &coll, nil
}

func deriveArrows(
	entries []domain.CollectionArrowEntry,
	collNS domain.Namespace,
) ([]domain.CollectionArrow, error) {
	bare := collNS.BareNamespace()
	arrows := make([]domain.CollectionArrow, 0, len(entries))
	for _, e := range entries {
		arrow, err := deriveArrow(e, bare)
		if err != nil {
			return nil, err
		}
		arrows = append(arrows, arrow)
	}
	return arrows, nil
}

func deriveArrow(e domain.CollectionArrowEntry, bare domain.Namespace) (domain.CollectionArrow, error) {
	if e.Namespace != "" {
		return domain.CollectionArrow{Namespace: domain.Namespace(e.Namespace)}, nil
	}
	segments := strings.Split(strings.TrimRight(e.Path, "/"), "/")
	last := segments[len(segments)-1]
	if last == "" {
		return domain.CollectionArrow{}, fmt.Errorf("manifold: arrow path %q produces an empty namespace segment", e.Path)
	}
	return domain.CollectionArrow{Namespace: domain.Namespace(string(bare) + "/" + last)}, nil
}
