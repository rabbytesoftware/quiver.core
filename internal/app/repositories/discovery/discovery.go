// Package discovery turns a query into verified arrows: it asks every provider
// for candidates, proves each one by fetching and parsing its manifest, and
// writes the proven ones to the vault.
package discovery

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

const defaultFetchConcurrency = 8

// Discovery searches git hosts for arrows and verifies what it finds.
type Discovery interface {
	// Discover streams every verified arrow to emit as it verifies, rather
	// than collecting and returning them. The Outcome summarises the pass,
	// including which providers failed and why; a provider failing is
	// reported there, not returned as an error.
	Discover(
		ctx context.Context,
		text string,
		emit func(Result),
	) (Outcome, error)
}

// KnownFn reports whether the catalog already holds a bare namespace. An error
// costs the caller the flag, never the result.
type KnownFn func(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error)

// Config holds the values read once at construction: the discovery markers, how
// many candidates to take from each provider, and how many manifest fetches may
// run at once. The fetch bound is politeness and local resource use, not a rate
// limit — manifests come off the raw CDN, not the metered search API.
type Config struct {
	Topics           []string
	PerProviderLimit int
	FetchConcurrency int
}

type discovery struct {
	providers   []provider.Provider
	manifold    manifold.Manifold
	vault       vault.Vault
	known       KnownFn
	topics      []string
	limit       int
	concurrency int
}

// New builds the pipeline. known may be nil, in which case nothing is ever
// flagged as already in the catalog.
func New(
	providers []provider.Provider,
	m manifold.Manifold,
	v vault.Vault,
	known KnownFn,
	cfg Config,
) (Discovery, error) {
	if m == nil {
		return nil, fmt.Errorf("discovery: no manifold configured")
	}
	if v == nil {
		return nil, fmt.Errorf("discovery: no vault configured")
	}

	concurrency := cfg.FetchConcurrency
	if concurrency <= 0 {
		concurrency = defaultFetchConcurrency
	}

	return &discovery{
		providers:   providers,
		manifold:    m,
		vault:       v,
		known:       known,
		topics:      cfg.Topics,
		limit:       cfg.PerProviderLimit,
		concurrency: concurrency,
	}, nil
}

func (d *discovery) Discover(
	ctx context.Context,
	text string,
	emit func(Result),
) (Outcome, error) {
	query := strings.TrimSpace(text)
	if query == "" {
		return Outcome{}, fmt.Errorf("discovery: query text is empty")
	}

	candidates, outcomes := d.search(ctx, query)
	unique := dedupe(candidates)

	counted := d.verify(ctx, unique, emit)

	return Outcome{
		Found:     len(unique),
		Verified:  counted.verified,
		Skipped:   counted.skipped,
		Providers: outcomes,
	}, nil
}

// search fans out to every provider at once and waits for all of them. A host
// that refuses contributes a ProviderOutcome instead of failing the pass.
func (d *discovery) search(
	ctx context.Context,
	query string,
) ([]provider.Candidate, []ProviderOutcome) {
	req := provider.SearchRequest{
		Text:   query,
		Topics: d.topics,
		Limit:  d.limit,
	}

	found := make([][]provider.Candidate, len(d.providers))
	outcomes := make([]ProviderOutcome, len(d.providers))

	var wg sync.WaitGroup
	for i, p := range d.providers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidates, err := p.Search(ctx, req)
			found[i] = candidates
			outcomes[i] = outcomeOf(p.Host(), candidates, err)
		}()
	}
	wg.Wait()

	return slices.Concat(found...), outcomes
}

// dedupe keeps the first candidate seen for each bare namespace: two hosts can
// answer for the same repository, and a repository is one arrow regardless of
// which ref a provider happened to name.
func dedupe(
	candidates []provider.Candidate,
) []provider.Candidate {
	seen := make(map[domain.Namespace]struct{}, len(candidates))
	unique := make([]provider.Candidate, 0, len(candidates))

	for _, candidate := range candidates {
		bare := candidate.Namespace.BareNamespace()
		if _, ok := seen[bare]; ok {
			continue
		}
		seen[bare] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

type tally struct {
	verified int
	skipped  int
}

type counters struct {
	mu    sync.Mutex
	tally tally
}

func (c *counters) add(
	verified bool,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if verified {
		c.tally.verified++
		return
	}
	c.tally.skipped++
}

func (c *counters) read() tally {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tally
}

// verify proves candidates with at most concurrency fetches in flight, emitting
// each one the moment it is proven so the caller never waits for the slowest.
func (d *discovery) verify(
	ctx context.Context,
	candidates []provider.Candidate,
	emit func(Result),
) tally {
	slots := make(chan struct{}, d.concurrency)
	stream := newStream(emit)

	var counted counters
	var wg sync.WaitGroup

	for _, candidate := range candidates {
		if !acquire(ctx, slots) {
			break
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			counted.add(d.verifyOne(ctx, candidate, stream))
		}()
	}

	wg.Wait()
	return counted.read()
}

// acquire takes a fetch slot, reporting false once the context is done. The
// explicit Err check comes first because a select whose cases are both ready
// picks at random, which would let a cancelled pass dispatch more work.
func acquire(
	ctx context.Context,
	slots chan struct{},
) bool {
	if ctx.Err() != nil {
		return false
	}

	select {
	case <-ctx.Done():
		return false
	case slots <- struct{}{}:
		return true
	}
}

// verifyOne fetches the candidate's manifest from the branch the search response
// named, indexes it, and streams it. It reports whether the candidate proved to
// be an arrow.
func (d *discovery) verifyOne(
	ctx context.Context,
	candidate provider.Candidate,
	stream *stream,
) bool {
	bare := candidate.Namespace.BareNamespace()

	arrow, raw, filename, err := d.manifold.ResolveArrow(ctx, bare.WithRef(candidate.DefaultBranch))
	if err != nil {
		return false
	}

	arrow.Namespace = bare.WithRef(arrow.Version)
	arrow.ResolvedBranch = candidate.DefaultBranch

	known := d.isKnown(ctx, arrow.Namespace)

	if err := d.index(ctx, arrow, raw, filename, candidate); err != nil {
		return false
	}

	stream.send(Result{
		Arrow:     *arrow,
		Namespace: bare,
		Stars:     candidate.Stars,
		Source:    candidate.Source,
		Known:     known,
	})
	return true
}

func (d *discovery) index(
	ctx context.Context,
	arrow *domain.Arrow,
	raw []byte,
	filename string,
	candidate provider.Candidate,
) error {
	return d.vault.PutArrow(ctx, arrow.Namespace, vault.ManifestFile{
		Content:  raw,
		Filename: filename,
		Meta: &vault.IndexMeta{
			Arrow:  arrow.ArrowMeta,
			OS:     supportedOS(arrow.Targets),
			Stars:  candidate.Stars,
			Source: candidate.Source,
			Branch: candidate.DefaultBranch,
		},
	})
}

// isKnown asks the stores rather than a previous response, so the flag is
// correct regardless of what the client did before. A lookup failure costs the
// flag, not the result.
func (d *discovery) isKnown(
	ctx context.Context,
	ns domain.Namespace,
) bool {
	if d.known != nil {
		if known, err := d.known(ctx, ns.BareNamespace()); err == nil && known {
			return true
		}
	}

	_, err := d.vault.GetArrow(ctx, ns)
	return err == nil
}

func supportedOS(
	targets map[domain.OS]domain.Target,
) []domain.OS {
	oses := make([]domain.OS, 0, len(targets))
	for os := range targets {
		oses = append(oses, os)
	}
	slices.Sort(oses)
	return oses
}
