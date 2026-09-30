package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

func (d *discovery) Browse(
	ctx context.Context,
	req BrowseRequest,
	emit func(Result),
) (Outcome, error) {
	if len(req.Sources) == 0 {
		return Outcome{}, fmt.Errorf("discovery: browse has no sources")
	}

	candidates, outcomes := d.browseSearch(ctx, req.Sources)
	unique := dedupe(candidates)
	plan := d.plan(ctx, unique, req.Budget)

	stream := newStream(emit)
	for _, hit := range plan.cached {
		stream.send(d.cachedResult(ctx, hit))
	}
	counted := d.resolvePending(ctx, plan, req.Want, emit)

	return Outcome{
		Found:     len(unique),
		Verified:  len(plan.cached) + counted.verified,
		Skipped:   plan.absent + counted.skipped,
		Providers: outcomes,
		Order:     plan.order,
	}, nil
}

// resolvePending resolves the new candidates in search order, stopping early
// once the vault's hits plus the newly proven ones reach want. A want of zero
// resolves every pending candidate.
func (d *discovery) resolvePending(
	ctx context.Context,
	plan browsePlan,
	want int,
	emit func(Result),
) tally {
	if want <= 0 {
		return d.verify(ctx, plan.pending, emit)
	}

	remaining := want - len(plan.cached)
	if remaining <= 0 {
		return tally{}
	}
	return d.verifyUntil(ctx, plan.pending, emit, remaining)
}

type cachedHit struct {
	candidate provider.Candidate
	arrow     *domain.Arrow
}

// classified is what the vault knew about one candidate: a parsed manifest
// within its TTL, a confirmed-absent marker, or nothing.
type classified struct {
	candidate provider.Candidate
	arrow     *domain.Arrow
	absent    bool
}

// browsePlan is what a pass will do with the candidates a search returned:
// which it can answer from the vault, which it must resolve, how many it drops
// as known misses, and the order the survivors came back in.
type browsePlan struct {
	cached  []cachedHit
	pending []provider.Candidate
	absent  int
	order   []domain.Namespace
}

// plan sorts candidates in search order into vault-fresh, known-absent and new,
// then keeps only as many new ones as the budget allows. The search order is
// the ranking, so the budget is spent on the best candidates first.
func (d *discovery) plan(
	ctx context.Context,
	unique []provider.Candidate,
	budget int,
) browsePlan {
	var plan browsePlan
	for _, candidate := range unique {
		plan.admit(d.classify(ctx, candidate), budget)
	}
	return plan
}

func (p *browsePlan) admit(
	entry classified,
	budget int,
) {
	bare := entry.candidate.Namespace.BareNamespace()

	if entry.arrow != nil {
		p.cached = append(p.cached, cachedHit{candidate: entry.candidate, arrow: entry.arrow})
		p.order = append(p.order, bare)
		return
	}
	if entry.absent {
		p.absent++
		return
	}
	if len(p.pending) >= budget {
		return
	}
	p.pending = append(p.pending, entry.candidate)
	p.order = append(p.order, bare)
}

// classify reports what the vault already knows about a candidate's repository.
// A parsed manifest under any ref means the repository was resolved within the
// TTL and costs nothing now; a confirmed-absent marker on the branch the search
// named means a recent attempt found nothing to install. The vault is asked for
// every ref, not just the branch: a repository drafted from a release tag is
// filed under that tag.
func (d *discovery) classify(
	ctx context.Context,
	candidate provider.Candidate,
) classified {
	bare := candidate.Namespace.BareNamespace()

	refs, _ := d.vault.ListVersions(ctx, bare)
	for _, ref := range refs {
		if arrow := d.freshArrow(ctx, bare.WithRef(ref)); arrow != nil {
			return classified{candidate: candidate, arrow: arrow}
		}
	}

	_, err := d.vault.GetArrow(ctx, bare.WithRef(candidate.DefaultBranch))
	return classified{candidate: candidate, absent: errors.Is(err, vault.ErrConfirmedAbsent)}
}

func (d *discovery) freshArrow(
	ctx context.Context,
	ns domain.Namespace,
) *domain.Arrow {
	file, err := d.vault.GetArrow(ctx, ns)
	if err != nil {
		return nil
	}

	arrow, err := d.manifold.ParseArrow(file.Content)
	if err != nil {
		return nil
	}
	arrow.Namespace = ns
	return arrow
}

func (d *discovery) cachedResult(
	ctx context.Context,
	hit cachedHit,
) Result {
	return Result{
		Arrow:     *hit.arrow,
		Namespace: hit.candidate.Namespace.BareNamespace(),
		Stars:     hit.candidate.Stars,
		Source:    hit.candidate.Source,
		InCatalog: d.inCatalog(ctx, hit.arrow.Namespace),
		InVault:   true,
	}
}

// browseSearch runs every source at once and keeps their candidates in source
// order, so a source listed first outranks one listed after it.
func (d *discovery) browseSearch(
	ctx context.Context,
	sources []BrowseSource,
) ([]provider.Candidate, []ProviderOutcome) {
	found := make([][]provider.Candidate, len(sources))
	outcomes := make([][]ProviderOutcome, len(sources))

	var wg sync.WaitGroup
	for i, source := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			found[i], outcomes[i] = d.browseSource(ctx, source)
		}()
	}
	wg.Wait()

	return flatten(found), flatten(outcomes)
}

func (d *discovery) browseSource(
	ctx context.Context,
	source BrowseSource,
) ([]provider.Candidate, []ProviderOutcome) {
	matching := d.providersFor(source.Host)
	if len(matching) == 0 {
		missing := ProviderOutcome{Host: source.Host, OK: true, Reason: ReasonUnsupported, Pass: PassBrowse}
		return nil, []ProviderOutcome{missing}
	}

	return d.searchOn(ctx, matching, provider.SearchRequest{
		Unmarked:     true,
		Sort:         source.Sort,
		MinStars:     source.MinStars,
		MaxStars:     source.MaxStars,
		PushedWithin: source.PushedWithin,
		Limit:        source.Limit,
	}, PassBrowse)
}

func (d *discovery) providersFor(
	host string,
) []provider.Provider {
	matching := make([]provider.Provider, 0, len(d.providers))
	for _, p := range d.providers {
		if p.Host() == host || strings.HasPrefix(p.Host(), host+".") {
			matching = append(matching, p)
		}
	}
	return matching
}

func flatten[T any](
	groups [][]T,
) []T {
	var all []T
	for _, group := range groups {
		all = append(all, group...)
	}
	return all
}

// recordAbsent remembers that a repository held nothing installable, so the
// next pass over it costs nothing until the vault's TTL lapses. Only a
// definitive not-found counts: a transient failure cached as absence would turn
// an outage into a miss that outlives it. Failing to write the marker costs the
// next pass the same live check, so it is logged and never propagated.
func (d *discovery) recordAbsent(
	ctx context.Context,
	ns domain.Namespace,
	resolveErr error,
) {
	if !errors.Is(resolveErr, manifoldresolver.ErrNotFound) {
		return
	}
	if err := d.vault.PutArrowNotFound(ctx, ns, ""); err != nil {
		slog.WarnContext(ctx, "discovery: record confirmed-absent result", "ns", ns, "err", err)
	}
}
