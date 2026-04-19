package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// Catalog manages the arrow read model: add, update, remove, list, get, and
// install-state queries. Projection subscriptions are registered on construction.
type Catalog interface {
	Add(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Update(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
	List(
		ctx context.Context,
	) ([]domain.Arrow, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	IsInstalled(
		ctx context.Context,
		ns domain.Namespace,
	) bool
	AddWithManifest(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.ArrowManifest,
	) error
	UpdateWithManifest(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.ArrowManifest,
	) error
}

type catalogService struct {
	axArrow         asynx.Asynx[domain.Arrow]
	axRuntime       asynx.Asynx[domainRuntime.ArrowRuntime]
	store           store.ArrowCatalog
	resolveManifest manifest.ResolveFunc
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	cat store.ArrowCatalog,
	resolve manifest.ResolveFunc,
) (Catalog, error) {
	c := &catalogService{
		axArrow:         axArrow,
		axRuntime:       axRuntime,
		store:           cat,
		resolveManifest: resolve,
	}

	if err := c.registerProjections(); err != nil {
		return nil, fmt.Errorf("catalog: register projections: %w", err)
	}

	return c, nil
}

func (c *catalogService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add arrow: %w", apperrors.ErrInvalidNamespace)
	}

	m, err := c.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("add arrow: %w: %w", apperrors.ErrFetchFailed, err)
	}

	version := manifestToVersion(m, ns.Ref(), true)

	if _, err := c.axArrow.Send(ctx, arrowcmds.AddArrow{
		Namespace:     ns,
		Version:       version,
		DirectInstall: true,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("add arrow: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("add arrow: %w", err)
	}

	return nil
}

func (c *catalogService) AddWithManifest(
	ctx context.Context,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add arrow with manifest: %w", apperrors.ErrInvalidNamespace)
	}

	version := manifestToVersion(manifest, ns.Ref(), false)

	if _, err := c.axArrow.Send(ctx, arrowcmds.AddArrow{
		Namespace:     ns,
		Version:       version,
		DirectInstall: false,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("add arrow with manifest: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("add arrow with manifest: %w", err)
	}

	return nil
}

func (c *catalogService) UpdateWithManifest(
	ctx context.Context,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
) error {
	version := manifestToVersion(manifest, ns.Ref(), false)

	if _, err := c.axArrow.Send(ctx, arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		Version:   version,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) || errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("update with manifest: %w", apperrors.ErrNotFound)
		}
		return fmt.Errorf("update with manifest: %w", err)
	}

	return nil
}

func (c *catalogService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := c.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("update arrow: %w", err)
	}
	if !exists {
		return fmt.Errorf("update arrow: %w", apperrors.ErrNotFound)
	}

	runtime, err := c.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return fmt.Errorf("update arrow: %w", err)
	}

	if runtime.Ref != "" && runtime.State != domain.ArrowStateReady {
		return fmt.Errorf("update arrow: %w", apperrors.ErrStateViolation)
	}

	m, err := c.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("update arrow: %w: %w", apperrors.ErrFetchFailed, err)
	}

	version := manifestToVersion(m, ns.Ref(), false)

	if _, err := c.axArrow.Send(ctx, arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		Version:   version,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("update arrow: %w", apperrors.ErrNotFound)
		}
		return fmt.Errorf("update arrow: %w", err)
	}

	return nil
}

func (c *catalogService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := c.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("remove arrow: %w", err)
	}
	if !exists {
		return fmt.Errorf("remove arrow: %w", apperrors.ErrNotFound)
	}

	runtime, err := c.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return fmt.Errorf("remove arrow: %w", err)
	}

	if runtime.Ref != "" {
		state := runtime.State
		if state != domain.ArrowStateAbsent && state != domain.ArrowStateRemoved && state != "" {
			return fmt.Errorf("remove arrow: %w", apperrors.ErrStateViolation)
		}
	}

	if err := c.axArrow.Forget(ctx, ns.String()); err != nil {
		return fmt.Errorf("remove arrow: %w", err)
	}

	// Best-effort: clean up runtime aggregate if it exists.
	if err := c.axRuntime.Forget(ctx, ns.String()); err != nil {
		slog.WarnContext(ctx, "remove arrow: runtime forget failed", "namespace", ns, "err", err)
	}

	return nil
}

func (c *catalogService) List(
	ctx context.Context,
) ([]domain.Arrow, error) {
	return c.store.List(ctx)
}

func (c *catalogService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	arrow, err := c.axArrow.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}

	return &arrow, nil
}

func (c *catalogService) IsInstalled(
	ctx context.Context,
	ns domain.Namespace,
) bool {
	rt, err := c.axRuntime.Get(ctx, ns.String())
	if err != nil {
		return false
	}
	return rt.Ref != "" && rt.State != domain.ArrowStateAbsent && rt.State != domain.ArrowStateRemoved
}

// manifestToVersion converts an ArrowManifest to a versioned ArrowManifest for command dispatch.
func manifestToVersion(
	manifest *domain.ArrowManifest,
	ref string,
	directInstall bool,
) domain.ArrowManifest {
	m := *manifest
	m.UserInstalled = directInstall
	m.InstalledRef = ref
	if m.InstalledAt.IsZero() {
		m.InstalledAt = time.Now()
	}
	return m
}
