package arrow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/repositories/arrow/internal/commands"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	gormdb "gorm.io/gorm"
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
	ResolveForInstall(
		ctx context.Context,
		ns domain.Namespace,
	) (
		resolvedNs domain.Namespace,
		arrow *domain.Arrow,
		constraint string,
		err error,
	)

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
	MarkInstalled(
		ctx context.Context,
		ns domain.Namespace,
		ref string,
		at time.Time,
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
}

func New(
	db *gormdb.DB,
	axArrow asynx.Asynx[domain.Arrow],
	v vault.Vault,
	m manifold.Manifold,
	hub apphub.WebSocketHub,
) (Arrow, error) {
	r, err := arrowstore.New(db, axArrow, v, m, hub)
	if err != nil {
		return nil, fmt.Errorf("catalog: store: %w", err)
	}

	if v != nil {
		if _, err := axArrow.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
			if delErr := v.DeleteWorkDir(ctx, evt.Aggregate.Namespace); delErr != nil {
				slog.WarnContext(ctx, "catalog: vault forget: delete work dir failed",
					"ns", evt.Aggregate.Namespace, "err", delErr)
			}
		}); err != nil {
			return nil, fmt.Errorf("catalog: vault forget projection: %w", err)
		}
	}

	return &arrowService{
		store:    r,
		axArrow:  axArrow,
		vault:    v,
		manifold: m,
	}, nil
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

func (s *arrowService) ResolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error) {
	return s.store.ResolveForInstall(ctx, ns)
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
		_, sendErr := s.axArrow.Send(ctx, arrowcmds.SetUserInstalled{Namespace: ns})
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
	_, sendErr := s.axArrow.Send(ctx, cmd)
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

	m, err := s.manifold.ParseArrow(data)
	if err != nil {
		return fmt.Errorf("seed arrow: %w: %w", apperrors.ErrInvalidManifest, err)
	}

	if ns.Ref() == "" && m.Version != "" {
		ns = ns.WithRef(m.Version)
	}

	if err := s.vault.PutArrow(ctx, ns, vault.ManifestFile{
		Content:  data,
		Filename: "ARROW.md"},
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
	_, err = s.axArrow.Send(ctx, cmd)
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

func (s *arrowService) MarkInstalled(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
	at time.Time,
) error {
	_, err := s.axArrow.Send(ctx, arrowcmds.MarkInstalled{
		Namespace:    ns,
		InstalledAt:  at,
		InstalledRef: ref,
	})
	return err
}

func (s *arrowService) Forget(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return s.axArrow.Forget(ctx, ns.String())
}

func (s *arrowService) UpdateManifest(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	_, err := s.axArrow.Send(ctx, arrowcmds.UpdateArrowManifest{
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

func (s *arrowService) OnArrowAdded(
	fn func(ctx context.Context, ns domain.Namespace, arrow domain.Arrow) error,
) error {
	_, err := s.axArrow.Subscribe(asynx.Topic("arrow.added.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domain.Arrow],
	) {
		if cbErr := fn(ctx, evt.Aggregate.Namespace, evt.Aggregate); cbErr != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowAdded failed",
				"ns", evt.Aggregate.Namespace, "err", cbErr)
		}
	})
	return err
}

func (s *arrowService) OnArrowUpdated(
	fn func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow) error,
) error {
	_, err := s.axArrow.Subscribe(asynx.Topic("arrow.updated.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domain.Arrow],
	) {
		if cbErr := fn(ctx, evt.Aggregate.Namespace, &evt.Aggregate); cbErr != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowUpdated failed",
				"ns", evt.Aggregate.Namespace, "err", cbErr)
		}
	})
	return err
}

func (s *arrowService) OnArrowRemoved(
	fn func(ctx context.Context, ns domain.Namespace) error,
) error {
	_, err := s.axArrow.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Arrow]) {
		if cbErr := fn(ctx, evt.Aggregate.Namespace); cbErr != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowRemoved failed",
				"ns", evt.Aggregate.Namespace, "err", cbErr)
		}
	})
	return err
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

	if !runtimeAlreadyExists {
		if delErr := s.vault.DeleteArrow(ctx, newNs); delErr != nil {
			slog.WarnContext(ctx, "upgrade version: delete pre-cached vault entry", "ns", newNs, "err", delErr)
		}
		if err := s.vault.RenameArrow(ctx, oldNs, newNs); err != nil {
			return nil, fmt.Errorf("upgrade version: rename vault entry: %w", err)
		}
		if err := s.vault.PutArrow(ctx, newNs, vault.ManifestFile{Content: rawBytes, Filename: filename}); err != nil {
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
	_, sendErr := s.axArrow.Send(ctx, cmd)
	if sendErr != nil {
		if errors.Is(sendErr, asynxModels.ErrValidation) || errors.Is(sendErr, asynxModels.ErrPipelineFailed) {
			return nil, fmt.Errorf("upgrade version: %w", apperrors.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("upgrade version: send command: %w", sendErr)
	}

	return newArrow, nil
}

func (s *arrowService) OnArrowUpgraded(
	fn func(ctx context.Context, arrow domain.Arrow) error,
) error {
	_, err := s.axArrow.Subscribe(asynx.Topic("arrow.upgraded.*"), func(
		ctx context.Context,
		evt asynxModels.Event[domain.Arrow],
	) {
		if cbErr := fn(ctx, evt.Aggregate); cbErr != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowUpgraded failed",
				"ns", evt.Aggregate.Namespace, "err", cbErr)
		}
	})
	return err
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
