package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/arrow/internal/store/internal/projections"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	gormdb "gorm.io/gorm"
)

type ResolveFunc func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)

type Store interface {
	List(
		ctx context.Context,
		userInstalled *bool,
	) ([]models.ArrowView, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
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
	) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error)
}

type storeService struct {
	db              storage.Store
	resolveManifest ResolveFunc
	manifold        manifold.Manifold
}

func New(
	db *gormdb.DB,
	axArrow asynx.Asynx[domain.Arrow],
	v vault.Vault,
	m manifold.Manifold,
	hub apphub.WebSocketHub,
) (Store, error) {
	st, err := storage.New(db)
	if err != nil {
		return nil, fmt.Errorf("store: storage: %w", err)
	}
	if err := projections.Register(st, axArrow, hub); err != nil {
		return nil, fmt.Errorf("store: projections: %w", err)
	}
	return &storeService{
		db:              st,
		resolveManifest: newResolver(v, m),
		manifold:        m,
	}, nil
}

func (r *storeService) List(
	ctx context.Context,
	userInstalled *bool,
) ([]models.ArrowView, error) {
	vms, err := r.db.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]models.ArrowView, 0, len(vms))
	for _, vm := range vms {
		view, err := r.toArrowView(ctx, vm, userInstalled)
		if err != nil {
			return nil, err
		}
		if view == nil {
			continue
		}
		result = append(result, *view)
	}

	return result, nil
}

func (r *storeService) toArrowView(
	ctx context.Context,
	vm storage.ViewModel,
	userInstalled *bool,
) (*models.ArrowView, error) {
	if userInstalled != nil && hasUserInstalled(vm.Versions) != *userInstalled {
		return nil, nil
	}

	versions, err := r.resolveVersionStates(ctx, vm.Versions)
	if err != nil {
		return nil, err
	}

	return &models.ArrowView{
		Namespace: vm.Namespace,
		Metadata:  vm.Metadata,
		Versions:  versions,
	}, nil
}

func (r *storeService) resolveVersionStates(
	_ context.Context,
	versions []storage.VersionRef,
) ([]models.VersionView, error) {
	result := make([]models.VersionView, 0, len(versions))
	for _, vr := range versions {
		result = append(result, models.VersionView{
			Namespace: vr.Namespace,
			Metadata:  vr.Metadata,
			State:     domain.ArrowStateAbsent,
		})
	}
	return result, nil
}

func (r *storeService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, fmt.Errorf("reader get: %w", err)
	}
	if vm == nil {
		return nil, apperrors.ErrNotFound
	}
	return &vm.Metadata, nil
}

func (r *storeService) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowDetailView, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, fmt.Errorf("reader get detail: %w", err)
	}
	if vm == nil {
		return nil, fmt.Errorf("reader get detail: %w", apperrors.ErrNotFound)
	}

	metadataArrow := vm.Metadata

	if ns.Ref() != "" {
		vr, found := findVersionRef(vm.Versions, ns)
		if !found {
			return nil, fmt.Errorf("reader get detail: %w", apperrors.ErrNotFound)
		}
		metadataArrow = vr.Metadata
	}

	return &models.ArrowDetailView{
		Metadata:   metadataArrow,
		State:      domain.ArrowStateAbsent,
		ActiveRun:  nil,
		LastReturn: nil,
	}, nil
}

func (r *storeService) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, fmt.Errorf("reader get manifest: %w", err)
	}
	if vm == nil {
		return nil, apperrors.ErrNotFound
	}

	if ns.Ref() == "" {
		return &vm.Metadata, nil
	}

	for _, vr := range vm.Versions {
		if vr.Namespace.String() == ns.String() {
			return &vr.Metadata, nil
		}
	}
	return nil, apperrors.ErrNotFound
}

func (r *storeService) ResolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	arrow, err := r.resolveManifest(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("reader resolve manifest: %w", err)
	}
	return arrow, nil
}

func (r *storeService) ResolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error) {
	resolvedNs = ns
	if ns.IsGlob() {
		constraint = ns.Ref()
		resolved, resolveErr := r.manifold.ResolveConstraint(ctx, ns, ns.Ref())
		if resolveErr != nil {
			return ns, nil, "", fmt.Errorf("reader resolve for install: %w", resolveErr)
		}
		resolvedNs = ns.WithRef(resolved)
	}

	arrow, err = r.resolveManifest(ctx, resolvedNs)
	if err != nil {
		return resolvedNs, nil, "", fmt.Errorf("reader resolve for install: %w", err)
	}

	return resolvedNs, arrow, constraint, nil
}

func findVersionRef(
	versions []storage.VersionRef,
	ns domain.Namespace,
) (storage.VersionRef, bool) {
	for _, vr := range versions {
		if vr.Namespace.String() == ns.String() {
			return vr, true
		}
	}
	return storage.VersionRef{}, false
}

func hasUserInstalled(
	versions []storage.VersionRef,
) bool {
	for _, v := range versions {
		if v.Metadata.UserInstalled {
			return true
		}
	}
	return false
}
