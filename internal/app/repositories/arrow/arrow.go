package arrow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

type Arrow interface {
	List(
		ctx context.Context,
		userInstalled *bool,
	) ([]models.ArrowView, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	Exists(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	GetDetail(
		ctx context.Context,
		ns domain.Namespace,
	) (*models.ArrowDetailView, error)
	GetManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	ResolveManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	RefreshManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	ResolveForInstall(
		ctx context.Context,
		ns domain.Namespace,
	) (
		resolvedNs domain.Namespace,
		arrow *domain.Arrow,
		constraint string,
		err error,
	)
	// ResolveCatalogued maps a namespace as the caller typed it onto the one
	// the catalog holds it under, so a refless namespace reaches the runtime
	// verbs as the ref they were catalogued with.
	ResolveCatalogued(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
	Search(
		ctx context.Context,
		q models.SearchQuery,
	) ([]models.CatalogHit, error)

	Add(
		ctx context.Context,
		ns domain.Namespace,
	) error
	AddDep(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
		constraint string,
	) error
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Seed(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error
	ValidateManifest(
		ctx context.Context,
		data []byte,
	) (*models.ValidationResult, error)
	// MarkInstalled records when the arrow's ref was installed.
	MarkInstalled(
		ctx context.Context,
		ns domain.Namespace,
		at time.Time,
	) error
	// MarkUninstalled clears the stamp MarkInstalled recorded.
	MarkUninstalled(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Forget(
		ctx context.Context,
		ns domain.Namespace,
	) error
	UpdateManifest(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error
	ResolveConstraint(
		ctx context.Context,
		ns domain.Namespace,
		constraint string,
	) (ref string, err error)
	UpgradeVersion(
		ctx context.Context,
		oldNs domain.Namespace,
		newNs domain.Namespace,
		constraint string,
		runtimeAlreadyExists bool,
	) (*domain.Arrow, error)
	Shutdown(
		ctx context.Context,
	) error

	// OnArrowUpdated carries the full *domain.Arrow so graph.SyncDependencies can be called directly without re-fetching.
	OnArrowAdded(fn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow domain.Arrow,
	) error) error
	OnArrowUpdated(fn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error) error
	OnArrowRemoved(fn func(
		ctx context.Context,
		ns domain.Namespace,
	) error) error
	// OnArrowUpgraded fires on arrow.upgraded.* events, carrying the new Arrow
	// with UpgradedFromNs set so reactions can coordinate old → new cleanup.
	OnArrowUpgraded(fn func(
		ctx context.Context,
		arrow domain.Arrow,
	) error) error
}

type arrowService struct {
	store    arrowstore.Store
	axArrow  asynx.Asynx[domain.Arrow]
	vault    vault.Vault
	manifold manifold.Manifold
	hub      apphub.WebSocketHub

	// asynx runs one goroutine per subscriber, so a second subscription on an
	// arrow topic would race the read-model write and the reactions alike.
	// Callbacks are held here and invoked by the single projection instead, in
	// the order the invariant needs.
	callbacksMu sync.RWMutex
	addedFns    []func(ctx context.Context, ns domain.Namespace, arrow domain.Arrow) error
	updatedFns  []func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow) error
	upgradedFns []func(ctx context.Context, arrow domain.Arrow) error
	removedFns  []func(ctx context.Context, ns domain.Namespace) error
}

func New(
	db *gormdb.DB,
	axArrow asynx.Asynx[domain.Arrow],
	v vault.Vault,
	m manifold.Manifold,
	hub apphub.WebSocketHub,
) (Arrow, error) {
	r, err := arrowstore.New(db, v, m)
	if err != nil {
		return nil, fmt.Errorf("catalog: store: %w", err)
	}

	s := &arrowService{
		store:    r,
		axArrow:  axArrow,
		vault:    v,
		manifold: m,
		hub:      hub,
	}

	if err := s.registerProjections(); err != nil {
		return nil, err
	}

	return s, nil
}

// registerProjections claims one subscriber per arrow topic. Everything an
// arrow event has to do — reactions, read model, broadcast — happens inside
// that subscriber, because asynx gives concurrent subscribers no order and the
// order is the whole point.
func (s *arrowService) registerProjections() error {
	topics := []struct {
		topic   string
		project asynxModels.ProjectionHandler[domain.Arrow]
	}{
		{"arrow.added.*", s.projectAdded},
		{"arrow.upgraded.*", s.projectUpgraded},
		{"arrow.updated.*", s.projectUpdated},
		{"arrow.installed.*", s.projectInstallStamp},
		{"arrow.uninstalled.*", s.projectInstallStamp},
	}

	for _, t := range topics {
		if _, err := s.axArrow.Subscribe(asynx.Topic(t.topic), t.project); err != nil {
			return fmt.Errorf("catalog projection: subscribe %s: %w", t.topic, err)
		}
	}

	if _, err := s.axArrow.OnForget(s.projectForgotten); err != nil {
		return fmt.Errorf("catalog projection: subscribe arrow forget: %w", err)
	}

	return nil
}

// project runs the reactions the arrow's usability depends on, then makes the
// arrow readable, then announces it.
//
// Dependency edges come first because they are what decides whether an arrow
// may be removed: an arrow readable in the catalog with no edges yet lets
// something it depends on be deleted out from under it. The broadcast comes
// last because a client told an arrow exists will go and read it.
//
// A read model that could not be written is never announced — announcing state
// nobody can read is the failure this ordering exists to prevent.
//
// A reaction that fails is logged and the arrow is still written. Nothing
// rebuilds the catalog from the event stream, so refusing the write would hide
// the arrow permanently, with no way left to even remove it; the next event for
// the same arrow re-runs the reaction.
func (s *arrowService) project(
	ctx context.Context,
	arrow domain.Arrow,
	react func(ctx context.Context, arrow domain.Arrow),
) {
	if react != nil {
		react(ctx, arrow)
	}

	if err := s.store.Project(ctx, arrow); err != nil {
		slog.ErrorContext(ctx, "catalog projection: write read model",
			"ns", arrow.Namespace, "err", err)
		return
	}

	s.broadcast(apphub.ArrowEvent{Kind: apphub.CatalogUpserted, Arrow: arrow})
}

func (s *arrowService) projectAdded(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, s.runAdded)
}

func (s *arrowService) projectUpdated(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, s.runUpdated)
}

func (s *arrowService) projectUpgraded(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, s.runUpgraded)
}

// projectInstallStamp carries the installed-ref stamp into the read model — set
// by an install, cleared by an uninstall. Nothing derived hangs off it, so there
// is no reaction to run first.
func (s *arrowService) projectInstallStamp(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, nil)
}

// projectForgotten mirrors project. The read-model row goes first: an arrow is
// readable only while its dependency edges exist, so the edges may only be
// dropped once nothing can read the arrow any more. The removal is announced
// last, once every trace of the arrow is gone.
func (s *arrowService) projectForgotten(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	arrow := evt.Aggregate

	if err := s.store.ProjectForget(ctx, arrow); err != nil {
		slog.ErrorContext(ctx, "catalog projection: delete from read model",
			"ns", arrow.Namespace, "err", err)
		return
	}

	s.runRemoved(ctx, arrow.Namespace)
	s.deleteWorkDir(ctx, arrow.Namespace)

	s.broadcast(apphub.ArrowEvent{Kind: apphub.CatalogRemoved, Arrow: arrow})
}

func (s *arrowService) broadcast(
	evt apphub.ArrowEvent,
) {
	if s.hub == nil {
		return
	}
	s.hub.BroadcastArrow(evt)
}

func (s *arrowService) deleteWorkDir(
	ctx context.Context,
	ns domain.Namespace,
) {
	if s.vault == nil {
		return
	}
	if err := s.vault.DeleteWorkDir(ctx, ns); err != nil {
		slog.WarnContext(ctx, "catalog: vault forget: delete work dir failed",
			"ns", ns, "err", err)
	}
}

func (s *arrowService) List(
	ctx context.Context,
	userInstalled *bool,
) ([]models.ArrowView, error) {
	return s.store.List(ctx, userInstalled)
}

func (s *arrowService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	return s.store.Get(ctx, ns)
}

func (s *arrowService) Exists(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	return s.axArrow.Exists(ctx, ns.String())
}

func (s *arrowService) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowDetailView, error) {
	return s.store.GetDetail(ctx, ns)
}

func (s *arrowService) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	return s.store.GetManifest(ctx, ns)
}

func (s *arrowService) ResolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	return s.store.ResolveManifest(ctx, ns)
}

// RefreshManifest purges the cached manifest, then resolves it — forcing a
// re-fetch from source rather than returning a still-fresh cached copy.
func (s *arrowService) RefreshManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if s.vault != nil {
		if err := s.vault.DeleteArrow(ctx, ns); err != nil {
			slog.WarnContext(ctx, "catalog: refresh: purge manifest cache failed",
				"ns", ns, "err", err)
		}
	}
	return s.store.ResolveManifest(ctx, ns)
}

func (s *arrowService) ResolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error) {
	return s.store.ResolveForInstall(ctx, ns)
}

func (s *arrowService) ResolveCatalogued(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, error) {
	return s.store.ResolveCatalogued(ctx, ns)
}

func (s *arrowService) Search(
	ctx context.Context,
	q models.SearchQuery,
) ([]models.CatalogHit, error) {
	return s.store.Search(ctx, q)
}

func (s *arrowService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	resolvedNs, arrow, constraint, err := s.store.ResolveForInstall(ctx, ns)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	arrow.UserInstalled = true
	arrow.InstalledConstraint = constraint
	return s.addArrowCommand(ctx, resolvedNs, arrow, constraint)
}

func (s *arrowService) AddDep(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	constraint string,
) error {
	return s.addArrowCommand(ctx, ns, arrow, constraint)
}

// addArrowCommand waits for the projections rather than only for the write.
// The caller's next move is to install the arrow, and installing reads the
// dependency edges this event produces; returning before they exist is what
// lets a still-depended-on arrow be removed.
func (s *arrowService) addArrowCommand(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	constraint string,
) error {
	existing, getErr := s.axArrow.Get(ctx, ns.String())
	if getErr == nil {
		if existing.UserInstalled {
			return nil
		}
		_, sendErr := s.axArrow.SendWait(ctx, arrowcmds.SetUserInstalled{Namespace: ns})
		return sendErr
	}
	if !errors.Is(getErr, asynxModels.ErrNotFound) {
		return fmt.Errorf("add arrow command: %w", getErr)
	}

	cmd := arrowcmds.AddArrow{
		Namespace:           ns,
		ArrowMeta:           arrow.ArrowMeta,
		Variables:           arrow.Variables,
		Netbridge:           arrow.Netbridge,
		Targets:             arrow.Targets,
		DirectInstall:       arrow.UserInstalled,
		InstalledConstraint: constraint,
	}
	_, sendErr := s.axArrow.SendWait(ctx, cmd)
	if sendErr == nil {
		return nil
	}
	if errors.Is(sendErr, asynxModels.ErrValidation) ||
		errors.Is(sendErr, asynxModels.ErrPipelineFailed) {
		return fmt.Errorf("add arrow: %w", apperrors.ErrAlreadyExists)
	}
	return fmt.Errorf("add arrow: %w", sendErr)
}

func (s *arrowService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := s.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	if !exists {
		return fmt.Errorf("remove: %w", apperrors.ErrNotFound)
	}

	return s.axArrow.Forget(ctx, ns.String())
}

func (s *arrowService) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("seed arrow: %w", apperrors.ErrInvalidNamespace)
	}
	// Seeded bytes have no remote to ask for a ref, so the caller has to say
	// which one these bytes are.
	if ns.Ref() == "" {
		return fmt.Errorf("seed arrow %s: namespace must carry a ref: %w", ns, apperrors.ErrInvalidNamespace)
	}

	m, err := s.manifold.ParseArrow(data)
	if err != nil {
		return fmt.Errorf("seed arrow: %w: %w", apperrors.ErrInvalidManifest, err)
	}

	// Cacheable, not a bare ManifestFile: seeded bytes are cached like any other
	// manifest, and a cache entry with no index metadata is one the vault lane of
	// search can never answer with.
	if err := s.vault.PutArrow(
		ctx, ns, arrowstore.Cacheable(m, data, "ARROW.md"),
	); err != nil {
		return fmt.Errorf("seed arrow: vault write: %w", err)
	}

	m.UserInstalled = true
	err = s.addArrowCommand(ctx, ns, m, "")
	if err == nil {
		return nil
	}
	if !errors.Is(err, apperrors.ErrAlreadyExists) {
		return fmt.Errorf("seed arrow: %w", err)
	}

	cmd := arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: m.ArrowMeta,
		Variables: m.Variables,
		Netbridge: m.Netbridge,
		Targets:   m.Targets,
	}
	_, err = s.axArrow.SendWait(ctx, cmd)
	return err
}

func (s *arrowService) ValidateManifest(
	ctx context.Context,
	data []byte,
) (*models.ValidationResult, error) {
	m, err := s.manifold.ParseArrow(data)
	if err == nil {
		return validManifestResult(m), nil
	}
	return invalidManifestResult(err), nil
}

// MarkInstalled stays on Send. It is called from inside a runtime projection,
// and waiting there would close the circular wait newAsynx documents
// (internal/app/container.go): an arrow worker blocked on axRuntime for the
// forget cascade while a runtime worker blocks on axArrow for this. Nothing
// reads the install stamp before the runtime reports the arrow ready anyway.
func (s *arrowService) MarkInstalled(
	ctx context.Context,
	ns domain.Namespace,
	at time.Time,
) error {
	_, err := s.axArrow.Send(ctx, arrowcmds.MarkInstalled{
		Namespace:   ns,
		InstalledAt: at,
	})
	return err
}

// MarkUninstalled stays on Send for the same reason MarkInstalled does: it is
// sent from the same runtime projection, so waiting here would close the same
// circular wait.
func (s *arrowService) MarkUninstalled(
	ctx context.Context,
	ns domain.Namespace,
) error {
	_, err := s.axArrow.Send(ctx, arrowcmds.MarkUninstalled{Namespace: ns})
	return err
}

func (s *arrowService) Forget(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return s.axArrow.Forget(ctx, ns.String())
}

// UpdateManifest waits for the projections: a changed manifest changes the
// dependency edges, and the caller reads them back straight away.
func (s *arrowService) UpdateManifest(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	_, err := s.axArrow.SendWait(ctx, arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: arrow.ArrowMeta,
		Variables: arrow.Variables,
		Netbridge: arrow.Netbridge,
		Targets:   arrow.Targets,
	})
	return err
}

func (s *arrowService) Shutdown(ctx context.Context) error {
	return s.axArrow.Shutdown(ctx)
}

// OnArrowAdded registers fn on the projection that owns arrow.added. Callbacks
// run in registration order, before the arrow becomes readable.
func (s *arrowService) OnArrowAdded(
	fn func(ctx context.Context, ns domain.Namespace, arrow domain.Arrow) error,
) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.addedFns = append(s.addedFns, fn)
	return nil
}

// OnArrowUpdated registers fn on the projection that owns arrow.updated.
func (s *arrowService) OnArrowUpdated(
	fn func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow) error,
) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.updatedFns = append(s.updatedFns, fn)
	return nil
}

// OnArrowRemoved registers fn on the forget projection. Callbacks run once the
// read-model row is gone, so nothing can read an arrow whose edges are being
// torn down.
func (s *arrowService) OnArrowRemoved(
	fn func(ctx context.Context, ns domain.Namespace) error,
) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.removedFns = append(s.removedFns, fn)
	return nil
}

func (s *arrowService) runAdded(
	ctx context.Context,
	arrow domain.Arrow,
) {
	for _, fn := range s.addedCallbacks() {
		if err := fn(ctx, arrow.Namespace, arrow); err != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowAdded failed",
				"ns", arrow.Namespace, "err", err)
		}
	}
}

func (s *arrowService) runUpdated(
	ctx context.Context,
	arrow domain.Arrow,
) {
	for _, fn := range s.updatedCallbacks() {
		if err := fn(ctx, arrow.Namespace, &arrow); err != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowUpdated failed",
				"ns", arrow.Namespace, "err", err)
		}
	}
}

func (s *arrowService) runUpgraded(
	ctx context.Context,
	arrow domain.Arrow,
) {
	for _, fn := range s.upgradedCallbacks() {
		if err := fn(ctx, arrow); err != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowUpgraded failed",
				"ns", arrow.Namespace, "err", err)
		}
	}
}

func (s *arrowService) runRemoved(
	ctx context.Context,
	ns domain.Namespace,
) {
	for _, fn := range s.removedCallbacks() {
		if err := fn(ctx, ns); err != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowRemoved failed",
				"ns", ns, "err", err)
		}
	}
}

// addedCallbacks and its siblings snapshot the registered callbacks so the
// projection never holds the lock while running them.
func (s *arrowService) addedCallbacks() []func(
	ctx context.Context,
	ns domain.Namespace,
	arrow domain.Arrow,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.addedFns)
}

func (s *arrowService) updatedCallbacks() []func(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.updatedFns)
}

func (s *arrowService) upgradedCallbacks() []func(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.upgradedFns)
}

func (s *arrowService) removedCallbacks() []func(
	ctx context.Context,
	ns domain.Namespace,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.removedFns)
}

func (s *arrowService) ResolveConstraint(
	ctx context.Context,
	ns domain.Namespace,
	constraint string,
) (ref string, err error) {
	return s.manifold.ResolveConstraint(ctx, ns, constraint)
}

func (s *arrowService) UpgradeVersion(
	ctx context.Context,
	oldNs domain.Namespace,
	newNs domain.Namespace,
	constraint string,
	runtimeAlreadyExists bool,
) (*domain.Arrow, error) {
	newArrow, rawBytes, filename, err := s.manifold.ResolveArrow(ctx, newNs)
	if err != nil {
		return nil, fmt.Errorf("upgrade version: fetch manifest: %w", err)
	}

	if !runtimeAlreadyExists { //nolint:nestif
		if delErr := s.vault.DeleteArrow(ctx, newNs); delErr != nil {
			slog.WarnContext(ctx, "upgrade version: delete pre-cached vault entry", "ns", newNs, "err", delErr)
		}
		if err := s.vault.RenameArrow(ctx, oldNs, newNs); err != nil {
			return nil, fmt.Errorf("upgrade version: rename vault entry: %w", err)
		}
		// Cacheable for the same reason Seed uses it: a bare ManifestFile carries
		// no index metadata, so the upgraded ref would be cached on disk yet
		// invisible to the vault lane of search.
		if err := s.vault.PutArrow(
			ctx, newNs, arrowstore.Cacheable(newArrow, rawBytes, filename),
		); err != nil {
			return nil, fmt.Errorf("upgrade version: write new manifest: %w", err)
		}
	}

	cmd := arrowcmds.UpgradeArrow{
		Namespace:           newNs,
		OldNamespace:        oldNs,
		ArrowMeta:           newArrow.ArrowMeta,
		Variables:           newArrow.Variables,
		Netbridge:           newArrow.Netbridge,
		Targets:             newArrow.Targets,
		InstalledConstraint: constraint,
	}
	// Send, not SendWait: the arrow.upgraded projection forgets the old
	// namespace (usecases/runtime.go onArrowUpgraded), which is itself a
	// blocking send on this same aggregate type. Waiting here would make one
	// arrow command depend on another completing.
	_, sendErr := s.axArrow.Send(ctx, cmd)
	if sendErr != nil {
		if errors.Is(sendErr, asynxModels.ErrValidation) || errors.Is(sendErr, asynxModels.ErrPipelineFailed) {
			return nil, fmt.Errorf("upgrade version: %w", apperrors.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("upgrade version: send command: %w", sendErr)
	}

	return newArrow, nil
}

// OnArrowUpgraded registers fn on the projection that owns arrow.upgraded.
func (s *arrowService) OnArrowUpgraded(
	fn func(ctx context.Context, arrow domain.Arrow) error,
) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.upgradedFns = append(s.upgradedFns, fn)
	return nil
}

func validManifestResult(
	m *domain.Arrow,
) *models.ValidationResult {
	supported := make([]domain.OS, 0, len(m.Targets))
	for os := range m.Targets {
		supported = append(supported, os)
	}
	unsupported := make([]domain.OS, 0)
	for _, os := range domain.AllOS() {
		if _, ok := m.Targets[os]; !ok {
			unsupported = append(unsupported, os)
		}
	}
	return &models.ValidationResult{
		Valid:                true,
		SupportedPlatforms:   supported,
		UnsupportedPlatforms: unsupported,
	}
}

func invalidManifestResult(
	err error,
) *models.ValidationResult {
	var asmErrs ruleset.RuleErrors
	if errors.As(err, &asmErrs) {
		errs := make([]models.ValidationError, len(asmErrs))
		for i, ae := range asmErrs {
			errs[i] = models.ValidationError{
				Field:   ae.Field,
				Rule:    ae.Rule,
				Message: ae.Message,
			}
		}
		return &models.ValidationResult{
			Valid:                false,
			Errors:               errs,
			SupportedPlatforms:   []domain.OS{},
			UnsupportedPlatforms: []domain.OS{},
		}
	}

	return &models.ValidationResult{
		Valid: false,
		Errors: []models.ValidationError{{
			Rule:    "parse_error",
			Message: err.Error(),
		}},
		SupportedPlatforms:   []domain.OS{},
		UnsupportedPlatforms: []domain.OS{},
	}
}
