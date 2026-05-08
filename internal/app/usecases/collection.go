package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	arrowrepo "github.com/rabbytesoftware/quiver/internal/app/repositories/arrow"
	quiverrepo "github.com/rabbytesoftware/quiver/internal/app/repositories/collection"
	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

type arrowCache interface {
	Seed(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error
	ResolveManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	GetManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
}

// CollectionUsecase is the public contract for collection operations.
type CollectionUsecase interface {
	Follow(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Unfollow(
		ctx context.Context,
		ns domain.Namespace,
	) error
	List(
		ctx context.Context,
		followed *bool,
	) ([]models.CollectionListDTO, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*models.CollectionDetailDTO, error)
	Seed(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error
	GetManifest(
		ctx context.Context,
		ns domain.Namespace,
	) ([]byte, error)
	ValidateManifest(
		ctx context.Context,
		data []byte,
	) (*models.ValidationResult, error)
}

type quiverUsecase struct {
	repo     quiverrepo.Collection
	arrows   arrowCache
	manifold manifold.Manifold
	vault    vault.Vault
}

func NewCollectionUsecase(
	repo quiverrepo.Collection,
	arrows arrowrepo.Arrow,
	m manifold.Manifold,
	v vault.Vault,
) CollectionUsecase {
	return &quiverUsecase{
		repo:     repo,
		arrows:   arrows,
		manifold: m,
		vault:    v,
	}
}

func withRetry(retries int, fn func() error) error {
	var err error
	for i := 0; i <= retries; i++ {
		if err = fn(); err == nil {
			return nil
		}
	}
	return err
}

func retryCount() int {
	cfg := config.GetArrows()
	if !cfg.AutoRetry.Enabled {
		return 0
	}
	return cfg.AutoRetry.Retries
}

func (u *quiverUsecase) Follow(
	ctx context.Context,
	ns domain.Namespace,
) error {
	coll, err := u.repo.Get(ctx, ns)
	if err != nil {
		return fmt.Errorf("follow collection: %w", err)
	}

	retries := retryCount()
	var failures []domain.Namespace
	for _, arrow := range coll.Arrows {
		arrowNS := arrow.Namespace
		var cacheErr error
		if arrow.IsLocal {
			cacheErr = withRetry(retries, func() error {
				_, b, _, e := u.manifold.ResolveArrow(ctx, arrowNS)
				if e != nil {
					return e
				}
				return u.arrows.Seed(ctx, arrowNS, b)
			})
		} else {
			cacheErr = withRetry(retries, func() error {
				_, e := u.arrows.ResolveManifest(ctx, arrowNS)
				return e
			})
		}
		if cacheErr != nil {
			failures = append(failures, arrowNS)
		}
	}

	return u.repo.Follow(ctx, ns, coll, failures)
}

func (u *quiverUsecase) Unfollow(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.repo.Unfollow(ctx, ns)
}

func (u *quiverUsecase) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*models.CollectionDetailDTO, error) {
	coll, err := u.repo.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get quiver: %w", err)
	}

	failedSet := make(map[domain.Namespace]struct{}, len(coll.FailedArrows))
	for _, failedNS := range coll.FailedArrows {
		failedSet[failedNS] = struct{}{}
	}

	arrows := make([]models.CollectionArrowDTO, len(coll.Arrows))
	for i, a := range coll.Arrows {
		dto := models.CollectionArrowDTO{Namespace: a.Namespace}
		if _, isFailed := failedSet[a.Namespace]; !isFailed {
			// GetManifest returns nil, ErrNotFound if not found; nil-check catches both errors and not-found
			arrowManifest, _ := u.arrows.GetManifest(ctx, a.Namespace)
			if arrowManifest != nil {
				dto.Resolved = true
				dto.Name = arrowManifest.Name
				dto.Version = arrowManifest.Version
				dto.Description = arrowManifest.Description
			}
		}
		arrows[i] = dto
	}

	followed, _ := u.repo.IsFollowed(ctx, ns)

	return &models.CollectionDetailDTO{
		Namespace:   ns,
		Name:        coll.Meta.Name,
		Version:     coll.Meta.Version,
		Description: coll.Meta.Description,
		URL:         coll.Meta.URL,
		Maintainers: coll.Meta.Maintainers,
		Tags:        coll.Meta.Tags,
		Media:       coll.Meta.Media,
		Arrows:      arrows,
		Followed:    followed,
	}, nil
}

func (u *quiverUsecase) List(
	ctx context.Context,
	followed *bool,
) ([]models.CollectionListDTO, error) {
	followedQuivers, err := u.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list quivers: %w", err)
	}

	if followed != nil && !*followed {
		return u.listUnfollowed(ctx, followedQuivers)
	}

	result := u.buildFollowedDTOs(followedQuivers)

	if followed == nil {
		unfollowed, unfollowedErr := u.listUnfollowed(ctx, followedQuivers)
		if unfollowedErr == nil {
			result = append(result, unfollowed...)
		}
	}

	return result, nil
}

func (u *quiverUsecase) buildFollowedDTOs(
	quivers []domain.Collection,
) []models.CollectionListDTO {
	result := make([]models.CollectionListDTO, 0, len(quivers))
	for _, q := range quivers {
		result = append(result, models.CollectionListDTO{
			Namespace:   q.Namespace,
			Name:        q.Meta.Name,
			Description: q.Meta.Description,
			Tags:        q.Meta.Tags,
			ArrowCount:  len(q.Arrows),
			Followed:    true,
		})
	}
	return result
}

func (u *quiverUsecase) listUnfollowed(
	ctx context.Context,
	followedQuivers []domain.Collection,
) ([]models.CollectionListDTO, error) {
	cached, err := u.vault.ListCachedCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list quivers: %w", err)
	}
	followedSet := make(map[domain.Namespace]struct{}, len(followedQuivers))
	for _, q := range followedQuivers {
		followedSet[q.Namespace] = struct{}{}
	}
	var result []models.CollectionListDTO
	for _, ns := range cached {
		if _, ok := followedSet[ns]; ok {
			continue
		}
		coll, getErr := u.repo.Get(ctx, ns)
		if getErr != nil {
			continue
		}
		result = append(result, models.CollectionListDTO{
			Namespace:   ns,
			Name:        coll.Meta.Name,
			Description: coll.Meta.Description,
			Tags:        coll.Meta.Tags,
			ArrowCount:  len(coll.Arrows),
			Followed:    false,
		})
	}
	return result, nil
}

func (u *quiverUsecase) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	coll, err := u.manifold.ParseCollection(data, ns)
	if err != nil {
		return fmt.Errorf("seed quiver: %w", err)
	}
	if _, err := u.vault.PutCollection(ctx, ns, coll); err != nil {
		return fmt.Errorf("seed quiver: %w", err)
	}
	return nil
}

func (u *quiverUsecase) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) ([]byte, error) {
	coll, err := u.repo.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get collection manifest: %w", err)
	}
	type arrowView struct {
		Namespace domain.Namespace `json:"namespace"`
	}
	type manifestView struct {
		Namespace domain.Namespace `json:"namespace"`
		Meta      domain.CollectionMeta `json:"meta"`
		Arrows    []arrowView      `json:"arrows"`
	}
	arrowViews := make([]arrowView, len(coll.Arrows))
	for i, a := range coll.Arrows {
		arrowViews[i].Namespace = a.Namespace
	}
	data, err := json.Marshal(manifestView{
		Namespace: coll.Namespace,
		Meta:      coll.Meta,
		Arrows:    arrowViews,
	})
	if err != nil {
		return nil, fmt.Errorf("get collection manifest: marshal: %w", err)
	}
	return data, nil
}

func (u *quiverUsecase) ValidateManifest(
	_ context.Context,
	data []byte,
) (*models.ValidationResult, error) {
	_, err := u.manifold.ParseCollection(data, "validation.dummy/collection")
	if err == nil {
		return &models.ValidationResult{
			Valid:                true,
			SupportedPlatforms:   []domain.OS{},
			UnsupportedPlatforms: []domain.OS{},
		}, nil
	}
	return invalidQuiverManifestResult(err), nil
}

func invalidQuiverManifestResult(err error) *models.ValidationResult {
	var ruleErrs ruleset.RuleErrors
	if errors.As(err, &ruleErrs) {
		errs := make([]models.ValidationError, len(ruleErrs))
		for i, e := range ruleErrs {
			errs[i] = models.ValidationError{
				Field:   e.Field,
				Rule:    e.Rule,
				Message: e.Message,
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
