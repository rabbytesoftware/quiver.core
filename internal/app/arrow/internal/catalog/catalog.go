package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
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
		arrow *domain.Arrow,
		directInstall bool,
		constraint string,
	) error
	Update(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
	// Retire removes a superseded version from the catalog unconditionally.
	// Unlike Remove, it does not check runtime state — it is only valid
	// after an upgrade has already renamed the vault entry away from ns.
	Retire(
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
	ListVersions(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Arrow, error)
}

type catalogService struct {
	axArrow   asynx.Asynx[domain.Arrow]
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	store     store.ArrowCatalog
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	cat store.ArrowCatalog,
) (Catalog, error) {
	c := &catalogService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		store:     cat,
	}

	if err := c.registerProjections(); err != nil {
		return nil, fmt.Errorf("catalog: register projections: %w", err)
	}

	return c, nil
}

func (c *catalogService) Add(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	directInstall bool,
	constraint string,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add arrow: %w", apperrors.ErrInvalidNamespace)
	}

	existing, getErr := c.axArrow.Get(ctx, ns.String())
	if getErr == nil {
		if directInstall && !existing.UserInstalled {
			_, sendErr := c.axArrow.Send(ctx, arrowcmds.SetUserInstalled{Namespace: ns})
			return sendErr
		}
		return nil
	}

	cmd := arrowcmds.AddArrow{
		Namespace:           ns,
		ArrowMeta:           arrow.ArrowMeta,
		Variables:           arrow.Variables,
		Netbridge:           arrow.Netbridge,
		Targets:             arrow.Targets,
		DirectInstall:       directInstall,
		InstalledConstraint: constraint,
	}
	if _, sendErr := c.axArrow.Send(ctx, cmd); sendErr != nil {
		if errors.Is(sendErr, asynxModels.ErrValidation) {
			return fmt.Errorf("add arrow: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("add arrow: %w", sendErr)
	}
	return nil
}

func (c *catalogService) Update(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
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

	cmd := arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: arrow.ArrowMeta,
		Variables: arrow.Variables,
		Netbridge: arrow.Netbridge,
		Targets:   arrow.Targets,
	}

	if _, err := c.axArrow.Send(ctx, cmd); err != nil {
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

	// Mid-install guard: MarkInstalled is best-effort — use runtime fallback.
	skipStateCheck := false
	arrow, arrowErr := c.axArrow.Get(ctx, ns.String())
	if arrowErr == nil && arrow.InstalledRef != "" && arrow.InstalledAt.IsZero() {
		rt, rtErr := c.axRuntime.Get(ctx, ns.String())
		if rtErr != nil || rt.State != domain.ArrowStateReady {
			return fmt.Errorf("remove arrow: %w", apperrors.ErrStateViolation)
		}
		// Runtime says ready — install completed, stamp was missed; allow remove.
		skipStateCheck = true
	}

	if !skipStateCheck {
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

func (c *catalogService) Retire(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := c.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("retire arrow: %w", err)
	}
	if !exists {
		return nil // already gone — idempotent
	}

	if err := c.axArrow.Forget(ctx, ns.String()); err != nil {
		return fmt.Errorf("retire arrow: %w", err)
	}

	// Best-effort: clean up runtime aggregate.
	if err := c.axRuntime.Forget(ctx, ns.String()); err != nil {
		slog.WarnContext(ctx, "retire arrow: runtime forget failed", "namespace", ns, "err", err)
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

func (c *catalogService) ListVersions(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Arrow, error) {
	return c.store.ListVersions(ctx, ns)
}
