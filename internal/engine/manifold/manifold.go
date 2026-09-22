package manifold

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
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

	// ResolveArrowAt fetches and validates an ArrowManifest at an explicit path
	// within its repository, skipping the owning-collection lookup ResolveArrow
	// performs for a quiver-hosted namespace. Use this when the caller already
	// knows the arrow's location — e.g. Follow, which just derived it from the
	// collection's own arrow list — to avoid re-resolving that same collection
	// once per local arrow.
	ResolveArrowAt(
		ctx context.Context,
		namespace domain.Namespace,
		path string,
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

	// ResolveLatestStable resolves a refless namespace to the ref of its latest
	// stable release, asking the host before listing git tags. It returns
	// ErrNoLatestStable when the repository publishes no stable release, which
	// is the caller's cue to fall back to a default branch.
	ResolveLatestStable(
		ctx context.Context,
		ns domain.Namespace,
	) (string, error)

	// ResolveLatestInChannel resolves a refless namespace to the ref of its
	// latest tag in the given channel. The "stable" channel reuses
	// ResolveLatestStable's exact algorithm, including its host-permalink
	// shortcut and its ErrNoLatestStable sentinel on a miss — no other
	// channel has a permalink, since a host's own "latest release" concept
	// only ever means "latest stable." Every other channel returns
	// ErrNoTagInChannel when no tag belongs to it.
	ResolveLatestInChannel(
		ctx context.Context,
		ns domain.Namespace,
		channel string,
	) (string, error)

	// ResolveDefaultBranch reports the branch a repository's HEAD points at,
	// and the commit hash that branch currently resolves to, read straight off
	// the git ref advertisement. It answers for every host, including
	// self-hosted and SSH remotes, and it names the branch the repository
	// actually defaults to rather than one guessed from a list.
	ResolveDefaultBranch(
		ctx context.Context,
		ns domain.Namespace,
	) (branch, hash string, err error)
}

// ErrNoLatestStable reports that a repository publishes no stable release, so
// no ref could be resolved for a refless namespace.
var ErrNoLatestStable = errors.New("manifold: no latest stable release")

// ErrInvalidManifest reports that manifest content — fetched or handed in
// directly — failed to become a valid domain.Arrow: bad YAML, a ruleset
// violation, or a compile/post-compile validation failure. It wraps every
// error ParseArrow returns, so a caller can tell "the content is bad" apart
// from a resolver-layer fetch failure without inspecting error text.
var ErrInvalidManifest = errors.New("manifold: invalid manifest")

// ErrArrowNotInCollection reports that a quiver-hosted namespace's AUID has
// no matching entry in its owning collection's current arrow list.
var ErrArrowNotInCollection = errors.New("manifold: arrow not found in its collection")

// StableChannel is the channel a tag belongs to when it carries no channel
// suffix at all.
const StableChannel = resolvers.StableChannel

// ErrNoTagInChannel reports that a repository has no tag classified into
// the requested channel.
var ErrNoTagInChannel = errors.New("manifold: no tag in channel")

// anyTag matches every tag, letting the constraint resolver rank the whole
// tag set instead of a subset.
const anyTag = "*"

type manifold struct {
	rsv        resolver.Resolver
	trs        translator.Translator
	cmp        compiler.Compiler
	rls        ruleset.Ruleset
	constraint resolvers.ConstraintResolver
	hosts      HostLookup
}

// New builds a Manifold that asks lookup whatever only a git host can answer.
// A nil lookup is a manifold that knows no hosts, which resolves every
// namespace by cloning it.
func New(
	fetchTimeout time.Duration,
	lookup HostLookup,
) Manifold {
	lookup = hosts.Or(lookup)

	return &manifold{
		rsv:        resolver.New(fetchTimeout, lookup),
		trs:        translator.NewTranslator(),
		cmp:        compiler.New(),
		rls:        ruleset.New(),
		constraint: resolvers.NewConstraintResolver(fetchTimeout),
		hosts:      lookup,
	}
}

// NewWithResolvers builds a Manifold with an injected resolver, constraint
// resolver and host lookup. Intended for tests that need to control how
// namespaces are resolved.
func NewWithResolvers(
	rsv resolver.Resolver,
	crs resolvers.ConstraintResolver,
	lookup HostLookup,
) Manifold {
	return &manifold{
		rsv:        rsv,
		trs:        translator.NewTranslator(),
		cmp:        compiler.New(),
		rls:        ruleset.New(),
		constraint: crs,
		hosts:      hosts.Or(lookup),
	}
}

func (m *manifold) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	raw, filename, err := m.resolveArrowBytes(ctx, namespace)
	if err != nil {
		return nil, nil, "", err
	}

	arrow, err := m.ParseArrow(raw)
	if err != nil {
		return nil, nil, "", err
	}

	return arrow, raw, filename, nil
}

func (m *manifold) ResolveArrowAt(
	ctx context.Context,
	namespace domain.Namespace,
	path string,
) (*domain.Arrow, []byte, string, error) {
	raw, filename, err := m.rsv.ResolveArrowAt(ctx, namespace, path)
	if err != nil {
		return nil, nil, "", err
	}

	arrow, err := m.ParseArrow(raw)
	if err != nil {
		return nil, nil, "", err
	}

	return arrow, raw, filename, nil
}

// resolveArrowBytes fetches an arrow manifest's raw bytes. A quiver-hosted
// (4-segment) namespace has no fixed on-disk location: its file can live
// anywhere in its owning collection's repository, so the owning collection
// is resolved first and its AUID looked up there. Every other namespace
// keeps the flat ARROW.md/arrow.yaml-at-root lookup.
func (m *manifold) resolveArrowBytes(
	ctx context.Context,
	namespace domain.Namespace,
) ([]byte, string, error) {
	if !namespace.BareNamespace().IsQuiverHosted() {
		return m.rsv.ResolveArrow(ctx, namespace)
	}

	path, err := m.resolveLocalArrowPath(ctx, namespace)
	if err != nil {
		return nil, "", err
	}
	return m.rsv.ResolveArrowAt(ctx, namespace, path)
}

func (m *manifold) resolveLocalArrowPath(
	ctx context.Context,
	namespace domain.Namespace,
) (string, error) {
	bare := namespace.BareNamespace()
	quid := domain.Namespace(bare.GetQUID()).WithRef(namespace.Ref())

	coll, err := m.ResolveCollection(ctx, quid)
	if err != nil {
		return "", fmt.Errorf("manifold: resolve owning collection %s for arrow %s: %w", quid, namespace, err)
	}

	for _, a := range coll.Arrows {
		if a.IsLocal && a.Namespace.BareNamespace() == bare {
			return a.SourcePath, nil
		}
	}

	return "", fmt.Errorf("manifold: arrow %s: %w", namespace, ErrArrowNotInCollection)
}

func (m *manifold) ParseArrow(
	data []byte,
) (*domain.Arrow, error) {
	module, err := m.trs.Arrow(data)
	if err != nil {
		return nil, fmt.Errorf("manifold: parse arrow: %w: %w", ErrInvalidManifest, err)
	}

	if readme, ok := m.trs.ExtractReadme(data); ok {
		module.Manifest.Readme = readme
	}

	if err := m.rls.ValidatePrecompile(module.Manifest, module.Precompiled); err != nil {
		return nil, fmt.Errorf("manifold: parse arrow: %w: %w", ErrInvalidManifest, err)
	}

	if err := m.cmp.Compile(module.Manifest, module.Precompiled, module.Selector); err != nil {
		return nil, fmt.Errorf("manifold: parse arrow: %w: %w", ErrInvalidManifest, err)
	}

	if err := m.rls.ValidateCompiled(module.Manifest); err != nil {
		return nil, fmt.Errorf("manifold: parse arrow: %w: %w", ErrInvalidManifest, err)
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

// ResolveLatestStable walks the chain release permalink → highest stable tag.
// The permalink step is an optimisation and never a requirement: any miss
// falls through, and a repository with no stable release reports
// ErrNoLatestStable rather than guessing.
func (m *manifold) ResolveLatestStable(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	if ref, ok := m.latestRelease(ctx, ns); ok {
		return ref, nil
	}

	ref, err := m.constraint.Resolve(ctx, ns, anyTag)
	if err != nil {
		return "", fmt.Errorf("manifold: latest stable %s: %w", ns, ErrNoLatestStable)
	}

	// A tag set with no semver member ranks lexicographically, so the winner
	// may be a prerelease. Only a stable tag answers this question.
	if !resolvers.IsStableSemver(ref) {
		return "", fmt.Errorf("manifold: latest stable %s: highest tag %q is not stable: %w", ns, ref, ErrNoLatestStable)
	}

	return ref, nil
}

func (m *manifold) ResolveLatestInChannel(
	ctx context.Context,
	ns domain.Namespace,
	channel string,
) (string, error) {
	if channel == StableChannel {
		return m.ResolveLatestStable(ctx, ns)
	}

	tags, err := m.constraint.ListTags(ctx, ns)
	if err != nil {
		return "", fmt.Errorf("manifold: latest in channel %s for %s: %w", channel, ns, err)
	}

	ref, ok := resolvers.LatestInChannel(tags, channel)
	if !ok {
		return "", fmt.Errorf("manifold: latest in channel %s for %s: %w", channel, ns, ErrNoTagInChannel)
	}
	return ref, nil
}

// latestRelease asks the host what it calls its latest release. A host that
// does not know, or is not known, is a miss: the tag listing answers for every
// host, so nothing here is worth failing over.
func (m *manifold) latestRelease(
	ctx context.Context,
	ns domain.Namespace,
) (string, bool) {
	host, ok := m.hosts(ns)
	if !ok {
		return "", false
	}

	ref, err := host.LatestRelease(ctx, ns)
	if err != nil || ref == "" {
		return "", false
	}
	return ref, true
}

func (m *manifold) ResolveDefaultBranch(
	ctx context.Context,
	ns domain.Namespace,
) (string, string, error) {
	branch, hash, err := m.constraint.DefaultBranch(ctx, ns)
	if err != nil {
		return "", "", fmt.Errorf("manifold: default branch %s: %w", ns, err)
	}
	return branch, hash, nil
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
	ref := collNS.Ref()
	arrows := make([]domain.CollectionArrow, 0, len(entries))
	for _, e := range entries {
		arrow, err := deriveArrow(e, bare, ref)
		if err != nil {
			return nil, err
		}
		arrows = append(arrows, arrow)
	}
	return arrows, nil
}

// deriveArrow settles a local member's namespace and on-disk location. The
// member lives inside the collection's own repository, at the collection's
// own commit, so its ref is the collection's — there is no other revision it
// could be at. Its identity (AUID) is the entry's explicit auid if given,
// else the last segment of its path — the path itself, in full, is kept as
// SourcePath so the file can live anywhere in the repository, not only at
// its root.
func deriveArrow(
	e domain.CollectionArrowEntry,
	bare domain.Namespace,
	ref string,
) (domain.CollectionArrow, error) {
	if e.Namespace != "" {
		return domain.CollectionArrow{Namespace: domain.Namespace(e.Namespace), IsLocal: false}, nil
	}

	// A ref authored on the path is the collection's ref restated; strip it
	// the same way Namespace itself would, rather than believe it.
	sourcePath := domain.Namespace(strings.Trim(e.Path, "/")).BareNamespace().String()

	auid := e.AUID
	if auid == "" {
		segments := strings.Split(sourcePath, "/")
		auid = segments[len(segments)-1]
	}
	if auid == "" {
		return domain.CollectionArrow{}, fmt.Errorf("manifold: arrow path %q produces an empty namespace segment", e.Path)
	}

	local := domain.Namespace(string(bare) + "/" + auid)
	return domain.CollectionArrow{
		Namespace:  local.WithRef(ref),
		IsLocal:    true,
		SourcePath: sourcePath,
	}, nil
}
