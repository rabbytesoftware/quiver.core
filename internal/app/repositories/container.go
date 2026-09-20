package repositories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	repoarrow "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/cascade"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection"
	repoconfig "github.com/rabbytesoftware/quiver.core/internal/app/repositories/config"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/device"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/pairingcode"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/app/selfarrow"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/core/shutdown"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	authdomain "github.com/rabbytesoftware/quiver.core/internal/domain/auth"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

type Container struct {
	Arrow       repoarrow.Arrow
	Runtime     runtime.Runtime
	Collection  collection.Collection
	Graph       graph.Graph
	Cascade     cascade.Cascade
	Discovery   discovery.Discovery
	Config      repoconfig.Config
	PairingCode pairingcode.PairingCode
	Device      device.Device
}

type repoOpts struct {
	selfUpdate *selfupdate.Trigger
}

// Option configures repositories.New.
type Option func(*repoOpts)

// WithSelfUpdateTrigger hands the container the process-lifetime trigger that
// quiver.core's own arrow fires when its update lifecycle succeeds. Without
// one, nothing watches for quiver.core's own update and the daemon keeps
// running the build it started as.
func WithSelfUpdateTrigger(
	trig *selfupdate.Trigger,
) Option {
	return func(o *repoOpts) { o.selfUpdate = trig }
}

func resolveOpts(
	opts []Option,
) repoOpts {
	cfg := repoOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	return cfg
}

func New(
	db *gormdb.DB,
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	axCollection asynx.Asynx[domain.Collection],
	collectionDBPath string,
	v vault.Vault,
	m manifold.Manifold,
	w wizardPkg.Wizard,
	os domain.OS,
	hub apphub.WebSocketHub,
	providers []provider.Provider,
	axPairingCode asynx.Asynx[authdomain.PairingCode],
	axDevice asynx.Asynx[authdomain.Device],
	deviceDB *gormdb.DB,
	opts ...Option,
) (*Container, error) {
	cat, err := repoarrow.New(db, axArrow, v, m, hub, preinstalledDetection(w, axRuntime, os)...)
	if err != nil {
		return nil, fmt.Errorf("repositories: arrow: %w", err)
	}

	// cat.ResolveManifest, not a raw asynx+manifold lookup: it is the same
	// vault-aware resolver GetManifest/GetReadme/GetDetail already resolve
	// through, so a manifest fetched once for one endpoint serves the graph
	// walk too instead of every /dependencies call paying its own live
	// manifold fetch for the same namespace.
	g, err := graph.New(db, os, m, cat.ResolveManifest)
	if err != nil {
		return nil, fmt.Errorf("repositories: graph: %w", err)
	}

	coll, err := collection.NewFromDBPath(axCollection, collectionDBPath, v, m)
	if err != nil {
		return nil, fmt.Errorf("repositories: quiver: %w", err)
	}

	rt, err := runtime.New(
		arrowGetter(axArrow),
		axRuntime,
		w,
		v,
		cat.MarkInstalled,
		cat.MarkUninstalled,
		cat.MarkLastUsed,
		dependentsChecker(g),
		catalogLister(cat),
		os,
	)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: runtime: %w", err)
	}

	fc, err := cascade.New(db, rt.Forget)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: cascade: %w", err)
	}

	disc, err := newDiscovery(providers, m, v, cat)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: discovery: %w", err)
	}

	pc, err := pairingcode.New(axPairingCode)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: pairingcode: %w", err)
	}

	dev, err := device.New(deviceDB, axDevice)
	if err != nil {
		discardCollection(coll)
		return nil, fmt.Errorf("repositories: device: %w", err)
	}

	c := &Container{
		Arrow:       cat,
		Runtime:     rt,
		Collection:  coll,
		Graph:       g,
		Cascade:     fc,
		Discovery:   disc,
		Config:      repoconfig.New(),
		PairingCode: pc,
		Device:      dev,
	}

	if err := c.wireCallbacks(resolveOpts(opts).selfUpdate); err != nil {
		discardCollection(coll)
		return nil, err
	}

	return c, nil
}

// preinstalledDetection wires Add-time preinstalled detection into the arrow
// repository, or nothing at all when there is no wizard to probe with.
//
// It is a direct, synchronous pair — probe then mark, both on the Add caller's
// own goroutine — rather than a reaction to arrow.added.*, which is this
// package's usual shape for a cross-repository consequence. Three things rule
// the reaction out here. The arrow.added event carries a domain.Arrow, which
// has no record of whether the add detected anything, so a reaction could only
// learn the answer by re-running the probe — spawning an arbitrary manifest
// command inside an asynx projection worker, blocking that shard for as long as
// it takes. A blocking runtime send from inside an arrow worker is also exactly
// the edge internal/app/container.go's newAsynx documents as one half of a
// cross-instance circular wait; the Add caller's goroutine is not a worker and
// blocks nobody. And the invariant is an ordering one: the runtime has to be
// Ready before the catalog row exists, which is before any arrow.added
// subscriber runs at all.
//
// The wizard is reached through Probe rather than Start: a preinstalled check
// is a question, not a supervised process, and Start would classify it as one
// that outlives the daemon's own shutdown.
func preinstalledDetection(
	w wizardPkg.Wizard,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	os domain.OS,
) []repoarrow.Option {
	if w == nil {
		return nil
	}

	return []repoarrow.Option{
		repoarrow.WithPreinstalledDetection(
			os,
			preinstalledProbe(w),
			runtime.MarkPreinstalled(axRuntime),
			runtime.ForgetPreinstalled(axRuntime),
		),
	}
}

// preinstalledProbe adapts the wizard's synchronous runner to the narrow
// function the arrow repository takes. There is no workdir and no PID: the
// namespace has no aggregate yet, so nothing has allocated either, and a check
// for software Quiver did not install has no use for the directory Quiver would
// have installed it into.
func preinstalledProbe(
	w wizardPkg.Wizard,
) repoarrow.PreinstalledProbeFn {
	return func(
		ctx context.Context,
		ns domain.Namespace,
		steps domainStep.StepList,
		vars map[string]string,
	) error {
		return w.Probe(ctx, wizardPkg.RunRequest{
			Namespace: ns,
			Variables: vars,
			Steps:     steps,
		})
	}
}

// arrowGetter hands the runtime a read of the arrow aggregate without handing
// it the aggregate.
func arrowGetter(
	axArrow asynx.Asynx[domain.Arrow],
) func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
	return func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		got, err := axArrow.Get(ctx, ns.String())
		if err != nil {
			return nil, err
		}
		return &got, nil
	}
}

// dependentsChecker asks the graph whether anything still needs ns. No arrow is
// excluded: the runtime asks on behalf of nobody in particular.
func dependentsChecker(
	g graph.Graph,
) runtime.HasDependentsFn {
	return func(ctx context.Context, ns domain.Namespace) (bool, error) {
		return g.HasDependents(ctx, ns, domain.Namespace(""))
	}
}

func catalogLister(
	cat repoarrow.Arrow,
) func(ctx context.Context) ([]models.ArrowView, error) {
	return func(ctx context.Context) ([]models.ArrowView, error) {
		return cat.List(ctx, nil)
	}
}

// newDiscovery reads the search settings once, per CLAUDE.md §15.2, and gives
// the pipeline a catalog lookup so an arrow the machine already knows is
// flagged rather than dropped. Discovery cannot verify anything without a
// manifold to parse with and a vault to write to, so a container built without
// them has no discovery rather than a half-built one.
func newDiscovery(
	providers []provider.Provider,
	m manifold.Manifold,
	v vault.Vault,
	cat repoarrow.Arrow,
) (discovery.Discovery, error) {
	if m == nil || v == nil {
		return nil, nil
	}

	search := config.GetSearch()

	return discovery.New(providers, m, v, catalogHas(cat), discovery.Config{
		Topics:           metadata.GetDiscovery().Topics,
		PerProviderLimit: search.PerProviderLimit,
		FetchConcurrency: search.FetchConcurrency,
	})
}

func catalogHas(
	cat repoarrow.Arrow,
) discovery.KnownFn {
	return func(ctx context.Context, ns domain.Namespace) (bool, error) {
		_, err := cat.Get(ctx, ns)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("catalog has %s: %w", ns, err)
	}
}

// discardCollection closes the collections database opened by NewFromDBPath when
// a later step of New fails, so a half-built container never leaves the file open
// with no owner.
func discardCollection(coll collection.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdown.DiscardTimeout)
	defer cancel()

	if err := coll.Shutdown(ctx); err != nil {
		slog.Warn("repositories: close collection store after failed construction", "err", err)
	}
}

// Shutdown drains every aggregate, blocking until in-flight commands have been
// persisted or ctx expires.
//
// Cascade drains first: its background goroutine calls Runtime.Forget, so it
// must finish (or be cut off) before Runtime itself shuts down underneath it.
//
// Runtime drains next and Arrow last. When an install finishes, the runtime
// reaction's onEnd writes arrow.MarkInstalled and only then commits
// EndExecution (runtime/internal/hooks.go). Draining Arrow first would lose
// MarkInstalled while EndExecution still commits, leaving a ready runtime whose
// arrow carries no installed ref — nothing reconciles that. Draining Runtime
// first makes EndExecution the write that fails instead, so the runtime stays
// in `installing`, which RecoverTransients re-drives on the next boot
// (runtime/internal/recovery.go).
//
// Every phase runs even when an earlier one fails, and each gets its own share of
// ctx rather than all three sharing it: an arrow whose process refuses to die
// makes the runtime drain spend the whole budget, and with a shared context
// Collection and Arrow would then be handed a dead one and skip their drain
// entirely — right before the adapters close the databases under them.
func (c *Container) Shutdown(ctx context.Context) error {
	return shutdown.Split(ctx, "repositories", []shutdown.Phase{
		{Name: "cascade shutdown", Run: c.Cascade.Shutdown},
		{Name: "runtime shutdown", Run: c.Runtime.Shutdown},
		{Name: "collection shutdown", Run: c.Collection.Shutdown},
		{Name: "arrow shutdown", Run: c.Arrow.Shutdown},
		{Name: "pairingcode shutdown", Run: c.PairingCode.Shutdown},
		{Name: "device shutdown", Run: c.Device.Shutdown},
	})
}

// RecoverForgetCascade finishes any forget cascade a prior crash left pending.
// Call once at boot, before anything else touches the namespaces involved —
// mirrors runtimeRepository.Start's use of RecoverTransients
// (runtime/internal/recovery.go) for the same crash-recovery role on the other
// side of an arrow removal.
func (c *Container) RecoverForgetCascade(ctx context.Context) {
	if err := c.Cascade.Drain(ctx); err != nil {
		slog.ErrorContext(ctx, "repositories: forget cascade recovery", "err", err)
	}
}

// wireCallbacks runs before any other registration, so the dependency graph is
// the first reaction to every arrow event. The arrow repository invokes
// callbacks in registration order and only makes the arrow readable afterwards,
// which is what makes "readable in the catalog" imply "its edges exist".
func (c *Container) wireCallbacks(
	trig *selfupdate.Trigger,
) error {
	if err := c.Arrow.OnArrowAdded(func(ctx context.Context, ns domain.Namespace, a domain.Arrow) error {
		return c.Graph.SyncDependencies(ctx, ns, &a)
	}); err != nil {
		return fmt.Errorf("repositories: wire OnArrowAdded: %w", err)
	}

	if err := c.Arrow.OnArrowUpdated(func(ctx context.Context, ns domain.Namespace, a *domain.Arrow) error {
		return c.Graph.SyncDependencies(ctx, ns, a)
	}); err != nil {
		return fmt.Errorf("repositories: wire OnArrowUpdated: %w", err)
	}

	// An upgrade replaces the manifest, so it replaces the edges too. graph no
	// longer projects this itself, and the usecase reaction that runs after
	// this one reads the edges back.
	if err := c.Arrow.OnArrowUpgraded(func(ctx context.Context, a domain.Arrow) error {
		return c.Graph.SyncDependencies(ctx, a.Namespace, &a)
	}); err != nil {
		return fmt.Errorf("repositories: wire OnArrowUpgraded: %w", err)
	}

	if err := c.Arrow.OnArrowRemoved(func(ctx context.Context, ns domain.Namespace) error {
		if err := c.Graph.RemoveDependencies(ctx, ns); err != nil {
			return err
		}
		return c.Cascade.Enqueue(ctx, ns)
	}); err != nil {
		return fmt.Errorf("repositories: wire OnArrowRemoved: %w", err)
	}

	return c.wireSelfUpdate(trig)
}

// wireSelfUpdate lets quiver.core's own update lifecycle claim this process.
// A container built without a trigger registers nothing at all: every test
// that builds one, and every command that is not the daemon, has no successor
// to hand over to.
func (c *Container) wireSelfUpdate(
	trig *selfupdate.Trigger,
) error {
	if trig == nil {
		return nil
	}

	if err := c.Runtime.OnRuntimeEnded(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		claimSuccession(trig, rt)
	}); err != nil {
		return fmt.Errorf("repositories: wire self-update trigger: %w", err)
	}

	return nil
}

// claimSuccession fires the trigger for the one execution that may replace the
// running daemon: quiver.core's own arrow, finishing its own update lifecycle,
// successfully. The namespace test carries the "@" for the same reason
// runtime/internal/recovery.go's does — without it, any namespace that merely
// starts with quiver.core's would pass.
//
// The resolved workdir is read from LastReturn rather than Execution because
// EndExecution clears Execution as it writes the return, so by the time this
// runs the execution that produced the binary is only visible through the
// variables it was resolved with.
func claimSuccession(
	trig *selfupdate.Trigger,
	rt domainRuntime.ArrowRuntime,
) {
	self, _ := metadata.GetSelfNamespaces()
	if !strings.HasPrefix(rt.Ref.String(), string(self)+"@") {
		return
	}
	if rt.LastReturn == nil || rt.LastReturn.Method != domain.MethodUpdate {
		return
	}
	if rt.LastReturn.Outcome != domainRuntime.ExecutionOutcomeSuccess {
		return
	}

	workdir := rt.LastReturn.Variables[domain.VarWorkdir]
	if workdir == "" {
		return
	}

	trig.Fire(filepath.Join(workdir, selfarrow.UpdatedBinaryName))
}

func (c *Container) RegisterHubProjections(hub apphub.WebSocketHub) error {
	if err := c.Runtime.OnRuntimeBegun(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeBegun: %w", err)
	}

	if err := c.Runtime.OnRuntimeEnded(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeEnded: %w", err)
	}

	if err := c.Runtime.OnRuntimeRecovered(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeRecovered: %w", err)
	}

	if err := c.Runtime.OnRuntimeDetached(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeDetached: %w", err)
	}

	if err := c.Runtime.OnRuntimePIDRecorded(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimePIDRecorded: %w", err)
	}

	if err := c.Runtime.OnRuntimeOutdated(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeOutdated: %w", err)
	}

	if err := c.Runtime.OnRuntimeOutdatedCleared(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeOutdatedCleared: %w", err)
	}

	if err := c.Runtime.OnRuntimeStepAdvanced(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		hub.BroadcastArrowRuntime(rt)
	}); err != nil {
		return fmt.Errorf("repositories: hub OnRuntimeStepAdvanced: %w", err)
	}

	if err := c.Collection.OnCollectionFollowed(func(_ context.Context, q domain.Collection) {
		hub.BroadcastCollection(apphub.CollectionEvent{Kind: apphub.CatalogUpserted, Collection: q})
	}); err != nil {
		return fmt.Errorf("repositories: hub OnCollectionFollowed: %w", err)
	}

	if err := c.Collection.OnCollectionUnfollowed(func(_ context.Context, ns domain.Namespace) {
		hub.BroadcastCollection(apphub.CollectionEvent{Kind: apphub.CatalogRemoved, Collection: domain.Collection{Namespace: ns}})
	}); err != nil {
		return fmt.Errorf("repositories: hub OnCollectionUnfollowed: %w", err)
	}

	return nil
}
