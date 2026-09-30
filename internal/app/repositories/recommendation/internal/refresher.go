package recommendationinternal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// sourcePool is how many candidates each source asks its host for: the page
// size, always. A search is one request whatever it returns, and most of the
// list is dropped cheaply (known absent) or never reached (the shelf fills
// first), so a wide pool costs nothing and a narrow one starves a shelf once
// its early candidates are cached as misses.
const sourcePool = 100

// Refresher rebuilds the snapshot of every shelf.
type Refresher interface {
	// Refresh rebuilds every shelf in turn. A shelf that fails keeps its previous
	// snapshot and does not stop the others; the errors are joined.
	Refresh(
		ctx context.Context,
	) error
}

// RefresherConfig is what a Refresher is built from.
type RefresherConfig struct {
	Browser    Browser
	Store      store.Store
	Shelves    []Shelf
	Budget     int
	MinEntries int
	Now        func() time.Time
	Hooks      *Hooks
}

type refresher struct {
	cfg RefresherConfig
}

// NewRefresher builds a Refresher.
func NewRefresher(
	cfg RefresherConfig,
) Refresher {
	return &refresher{cfg: cfg}
}

func (r *refresher) Refresh(
	ctx context.Context,
) error {
	var errs []error
	replaced := 0

	for _, shelf := range r.cfg.Shelves {
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("recommendation: refresh: %w", err))
			break
		}
		swapped, err := r.refreshShelf(ctx, shelf)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if swapped {
			replaced++
		}
	}

	if replaced > 0 {
		r.cfg.Hooks.Fire(ctx)
	}
	return errors.Join(errs...)
}

// refreshShelf reports whether it replaced the shelf. A pass that found nothing
// for a shelf that already has a snapshot leaves that snapshot alone.
func (r *refresher) refreshShelf(
	ctx context.Context,
	shelf Shelf,
) (bool, error) {
	found, err := r.browse(ctx, shelf)
	if err != nil {
		return false, fmt.Errorf("recommendation: refresh shelf %s: %w", shelf.ID, err)
	}

	previous, err := r.cfg.Store.Snapshot(ctx, shelf.ID)
	if err != nil {
		return false, fmt.Errorf("recommendation: refresh shelf %s: %w", shelf.ID, err)
	}

	entries := padded(found, previous.Entries, r.cfg.MinEntries)
	if len(entries) == 0 && len(previous.Entries) > 0 {
		return false, nil
	}
	if err := r.cfg.Store.Replace(ctx, shelf.ID, r.cfg.Now(), entries); err != nil {
		return false, fmt.Errorf("recommendation: refresh shelf %s: %w", shelf.ID, err)
	}
	return true, nil
}

// browse asks discovery for one shelf and returns its entries in search order.
// A cancelled pass and a pass no provider answered both return an error, so the
// caller never swaps a shelf for an empty list that only reflects an outage.
func (r *refresher) browse(
	ctx context.Context,
	shelf Shelf,
) ([]store.Entry, error) {
	var collected results

	outcome, err := r.cfg.Browser.Browse(ctx, request(shelf, r.cfg.Budget), collected.add)
	if err != nil {
		return nil, fmt.Errorf("browse: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("browse: %w", err)
	}
	if noneAnswered(outcome) {
		return nil, errors.New("browse: no provider answered")
	}
	return collected.ordered(outcome.Order, shelf.Limit), nil
}

func request(
	shelf Shelf,
	budget int,
) discovery.BrowseRequest {
	sources := make([]discovery.BrowseSource, 0, len(shelf.Sources))
	for _, source := range shelf.Sources {
		sources = append(sources, discovery.BrowseSource{
			Host:         source.Host,
			Sort:         source.Sort,
			MinStars:     source.MinStars,
			MaxStars:     source.MaxStars,
			PushedWithin: source.PushedWithin,
			Limit:        sourcePool,
		})
	}
	return discovery.BrowseRequest{Sources: sources, Budget: budget, Want: shelf.Limit}
}

func noneAnswered(
	outcome discovery.Outcome,
) bool {
	if len(outcome.Providers) == 0 {
		return false
	}
	for _, provider := range outcome.Providers {
		if provider.OK {
			return false
		}
	}
	return true
}

// padded tops a short list up to min entries from the shelf's previous
// snapshot, best first, skipping arrows the new list already has. A list that is
// long enough is returned as it is.
func padded(
	found []store.Entry,
	previous []store.Entry,
	minEntries int,
) []store.Entry {
	if len(found) >= minEntries {
		return found
	}

	present := make(map[string]struct{}, len(found))
	for _, entry := range found {
		present[entry.Namespace] = struct{}{}
	}

	out := append([]store.Entry{}, found...)
	for _, entry := range previous {
		if len(out) >= minEntries {
			break
		}
		if _, ok := present[entry.Namespace]; ok {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// results gathers what a browse pass emits, which arrives as candidates verify
// and in no particular order.
type results struct {
	mu    sync.Mutex
	found map[domain.Namespace]discovery.Result
}

func (r *results) add(
	result discovery.Result,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.found == nil {
		r.found = make(map[domain.Namespace]discovery.Result)
	}
	r.found[result.Namespace] = result
}

// ordered lays the gathered results out in the order the hosts ranked them,
// keeping the first limit.
func (r *results) ordered(
	order []domain.Namespace,
	limit int,
) []store.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries := make([]store.Entry, 0, min(len(order), limit))
	for _, ns := range order {
		result, ok := r.found[ns]
		if !ok {
			continue
		}
		entries = append(entries, store.Entry{Namespace: ns.String(), Stars: result.Stars, Source: result.Source})
		if len(entries) == limit {
			break
		}
	}
	return entries
}
