// Package recommendation keeps the query-less shelves the desktop home shows. A
// refresh searches each shelf's sources through discovery and swaps the result
// in as a snapshot; reading a shelf never reaches a git host.
package recommendation

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	recommendationinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// Recommendation serves the home shelves and keeps them fresh.
type Recommendation interface {
	// Home returns every configured shelf from its snapshot, in configuration
	// order. It reads the local store and the vault index only. Disabled, it
	// returns no shelves.
	Home(
		ctx context.Context,
	) ([]Shelf, error)

	// Refresh starts a refresh in the background and returns at once, or joins
	// the one already running. Disabled, it does nothing.
	Refresh(
		ctx context.Context,
	)

	// Refreshing reports whether a refresh is running.
	Refreshing() bool

	// Start launches the scheduler, which refreshes once now if any shelf is
	// missing or older than the interval and then every interval, until ctx is
	// cancelled. It returns at once. Disabled, it does nothing.
	Start(
		ctx context.Context,
	)

	// Shutdown stops the scheduler and any refresh in flight.
	Shutdown(
		ctx context.Context,
	) error

	// OnHomeRefreshed registers a callback fired after a refresh replaced at
	// least one shelf.
	OnHomeRefreshed(
		fn func(ctx context.Context),
	) error
}

type recommendation struct {
	enabled   bool
	interval  time.Duration
	shelves   []recommendationinternal.Shelf
	store     store.Store
	vault     vault.Vault
	known     discovery.KnownFn
	now       func() time.Time
	hooks     *recommendationinternal.Hooks
	scheduler recommendationinternal.Scheduler
}

type options struct {
	now func() time.Time
}

// Option configures New.
type Option func(*options)

// WithClock overrides the clock used to stamp and age snapshots.
func WithClock(
	now func() time.Time,
) Option {
	return func(o *options) {
		if now != nil {
			o.now = now
		}
	}
}

// New builds the repository over db, which holds the snapshot read model.
// known may be nil, in which case nothing is flagged as already in the catalog.
func New(
	db *gorm.DB,
	disc discovery.Discovery,
	v vault.Vault,
	known discovery.KnownFn,
	cfg Config,
	opts ...Option,
) (Recommendation, error) {
	if disc == nil {
		return nil, fmt.Errorf("recommendation: no discovery configured")
	}
	if v == nil {
		return nil, fmt.Errorf("recommendation: no vault configured")
	}

	cfgOpts := options{now: time.Now}
	for _, opt := range opts {
		opt(&cfgOpts)
	}

	st, err := store.New(db)
	if err != nil {
		return nil, fmt.Errorf("recommendation: %w", err)
	}

	r := &recommendation{
		enabled: cfg.Enabled,
		store:   st,
		vault:   v,
		known:   known,
		now:     cfgOpts.now,
		hooks:   &recommendationinternal.Hooks{},
	}
	if !cfg.Enabled {
		return r, nil
	}

	if err := r.enable(disc, cfg); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *recommendation) enable(
	disc discovery.Discovery,
	cfg Config,
) error {
	interval, err := time.ParseDuration(cfg.RefreshInterval)
	if err != nil || interval <= 0 {
		return fmt.Errorf("recommendation: refresh interval %q: want a positive duration", cfg.RefreshInterval)
	}

	r.interval = interval
	r.shelves = buildShelves(cfg.Shelves)

	refresher := recommendationinternal.NewRefresher(recommendationinternal.RefresherConfig{
		Browser:    disc,
		Store:      r.store,
		Shelves:    r.shelves,
		Budget:     cfg.CandidateBudget,
		MinEntries: cfg.MinEntries,
		Now:        r.now,
		Hooks:      r.hooks,
	})
	r.scheduler = recommendationinternal.NewScheduler(recommendationinternal.SchedulerConfig{
		Run:      r.run(refresher),
		Due:      r.due,
		Interval: interval,
	})
	return nil
}

func (r *recommendation) run(
	refresher recommendationinternal.Refresher,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if err := r.store.Retain(ctx, r.shelfIDs()); err != nil {
			return fmt.Errorf("recommendation: %w", err)
		}
		return refresher.Refresh(ctx)
	}
}

func (r *recommendation) shelfIDs() []string {
	ids := make([]string, 0, len(r.shelves))
	for _, shelf := range r.shelves {
		ids = append(ids, shelf.ID)
	}
	return ids
}

func (r *recommendation) Refresh(
	ctx context.Context,
) {
	if !r.enabled {
		return
	}
	r.scheduler.Trigger(ctx)
}

func (r *recommendation) Refreshing() bool {
	return r.enabled && r.scheduler.Running()
}

func (r *recommendation) Start(
	ctx context.Context,
) {
	if !r.enabled {
		return
	}
	r.scheduler.Start(ctx)
}

func (r *recommendation) Shutdown(
	ctx context.Context,
) error {
	if !r.enabled {
		return nil
	}
	return r.scheduler.Shutdown(ctx)
}

func (r *recommendation) OnHomeRefreshed(
	fn func(ctx context.Context),
) error {
	return r.hooks.OnRefreshed(fn)
}

// due reports whether any shelf has never been filled or has aged past the
// interval. A store that cannot be read counts as due: the refresh will report
// the same failure where it can be logged.
func (r *recommendation) due(
	ctx context.Context,
) bool {
	for _, shelf := range r.shelves {
		snap, err := r.store.Snapshot(ctx, shelf.ID)
		if err != nil || snap.RefreshedAt.IsZero() || r.now().Sub(snap.RefreshedAt) >= r.interval {
			return true
		}
	}
	return false
}

func (r *recommendation) Home(
	ctx context.Context,
) ([]Shelf, error) {
	shelves := make([]Shelf, 0, len(r.shelves))
	for _, shelf := range r.shelves {
		read, err := r.readShelf(ctx, shelf)
		if err != nil {
			return nil, err
		}
		shelves = append(shelves, read)
	}
	return shelves, nil
}

func (r *recommendation) readShelf(
	ctx context.Context,
	shelf recommendationinternal.Shelf,
) (Shelf, error) {
	snap, err := r.store.Snapshot(ctx, shelf.ID)
	if err != nil {
		return Shelf{}, fmt.Errorf("recommendation: home: %w", err)
	}

	entries := make([]Entry, 0, len(snap.Entries))
	for _, stored := range snap.Entries {
		entry, ok, err := r.entry(ctx, stored)
		if err != nil {
			return Shelf{}, fmt.Errorf("recommendation: home: shelf %s: %w", shelf.ID, err)
		}
		if ok {
			entries = append(entries, entry)
		}
	}
	return Shelf{ID: shelf.ID, Title: shelf.Title, RefreshedAt: snap.RefreshedAt, Entries: entries}, nil
}

// entry attaches the vault's index rows to a stored entry. It reports false for
// an arrow whose rows have expired or been forgotten, which a shelf skips rather
// than show as an empty card.
func (r *recommendation) entry(
	ctx context.Context,
	stored store.Entry,
) (Entry, bool, error) {
	ns := domain.Namespace(stored.Namespace)

	rows, err := r.vault.SearchArrows(ctx, vault.IndexQuery{Text: stored.Namespace, Limit: indexLookupLimit})
	if err != nil {
		return Entry{}, false, fmt.Errorf("vault index %s: %w", ns, err)
	}

	own := make([]vault.IndexRow, 0, len(rows))
	for _, row := range rows {
		if row.Namespace.BareNamespace() == ns {
			own = append(own, row)
		}
	}
	if len(own) == 0 {
		return Entry{}, false, nil
	}

	return Entry{
		Namespace: ns,
		Stars:     stored.Stars,
		Source:    stored.Source,
		Rows:      own,
		InCatalog: r.inCatalog(ctx, ns),
	}, true, nil
}

// indexLookupLimit bounds the arrows a namespace lookup scans: the search is a
// substring match, so a namespace that is the prefix of others matches them too.
const indexLookupLimit = 25

// inCatalog asks the catalog at read time so the flag is right whatever the
// user did since the last refresh. A lookup failure costs the flag, not the
// entry.
func (r *recommendation) inCatalog(
	ctx context.Context,
	ns domain.Namespace,
) bool {
	if r.known == nil {
		return false
	}
	known, err := r.known(ctx, ns)
	return err == nil && known
}
