package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	arrowrepo "github.com/rabbytesoftware/quiver/internal/app/repositories/arrow"
	quiverrepo "github.com/rabbytesoftware/quiver/internal/app/repositories/quiver"
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

// QuiverUsecase is the public contract for quiver operations.
type QuiverUsecase interface {
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
	) ([]models.QuiverListDTO, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*models.QuiverDetailDTO, error)
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
}

type quiverUsecase struct {
	repo     quiverrepo.Quiver
	arrows   arrowCache
	manifold manifold.Manifold
	vault    vault.Vault
}

func NewQuiverUsecase(
	repo quiverrepo.Quiver,
	arrows arrowrepo.Arrow,
	m manifold.Manifold,
	v vault.Vault,
) QuiverUsecase {
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
	manifest, localBytes, err := u.repo.Get(ctx, ns)
	if err != nil {
		return fmt.Errorf("follow quiver: %w", err)
	}

	retries := retryCount()
	var failures []domain.Namespace
	for _, arrow := range manifest.Arrows {
		arrowNS := arrow.Namespace
		var cacheErr error
		if bytes, ok := localBytes[arrowNS]; ok {
			cacheErr = withRetry(retries, func() error {
				return u.arrows.Seed(ctx, arrowNS, bytes)
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

	if len(failures) > 0 {
		_ = u.repo.UpdateFailedArrows(ctx, ns, failures)
	}

	return u.repo.Follow(ctx, ns)
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
) (*models.QuiverDetailDTO, error) {
	manifest, _, err := u.repo.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get quiver: %w", err)
	}

	retries := retryCount()
	arrows := make([]models.QuiverArrowDTO, len(manifest.Arrows))
	for i, a := range manifest.Arrows {
		dto := models.QuiverArrowDTO{Namespace: a.Namespace}
		var arrowManifest *domain.Arrow
		enrichErr := withRetry(retries, func() error {
			var e error
			arrowManifest, e = u.arrows.GetManifest(ctx, a.Namespace)
			return e
		})
		if enrichErr == nil && arrowManifest != nil {
			dto.Resolved = true
			dto.Name = arrowManifest.Name
			dto.Version = arrowManifest.Version
			dto.Description = arrowManifest.Description
		}
		arrows[i] = dto
	}

	followed, _ := u.repo.IsFollowed(ctx, ns)

	return &models.QuiverDetailDTO{
		Namespace:   ns,
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		URL:         manifest.URL,
		Maintainers: manifest.Maintainers,
		Tags:        manifest.Tags,
		Media:       manifest.Media,
		Arrows:      arrows,
		Followed:    followed,
	}, nil
}

func (u *quiverUsecase) List(
	ctx context.Context,
	followed *bool,
) ([]models.QuiverListDTO, error) {
	followedQuivers, err := u.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list quivers: %w", err)
	}

	if followed != nil && !*followed {
		return u.listUnfollowed(ctx, followedQuivers)
	}

	result, err := u.buildFollowedDTOs(ctx, followedQuivers)
	if err != nil {
		return nil, err
	}

	if followed == nil {
		unfollowed, unfollowedErr := u.listUnfollowed(ctx, followedQuivers)
		if unfollowedErr == nil {
			result = append(result, unfollowed...)
		}
	}

	return result, nil
}

func (u *quiverUsecase) buildFollowedDTOs(
	ctx context.Context,
	quivers []domain.Quiver,
) ([]models.QuiverListDTO, error) {
	result := make([]models.QuiverListDTO, 0, len(quivers))
	for _, q := range quivers {
		manifest, _, err := u.repo.Get(ctx, q.Namespace)
		if err != nil {
			continue
		}
		result = append(result, models.QuiverListDTO{
			Namespace:   q.Namespace,
			Name:        manifest.Name,
			Description: manifest.Description,
			Tags:        manifest.Tags,
			ArrowCount:  len(manifest.Arrows),
			Followed:    true,
		})
	}
	return result, nil
}

func (u *quiverUsecase) listUnfollowed(
	ctx context.Context,
	followedQuivers []domain.Quiver,
) ([]models.QuiverListDTO, error) {
	cached, err := u.vault.ListCachedQuivers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list quivers: %w", err)
	}
	followedSet := make(map[domain.Namespace]struct{}, len(followedQuivers))
	for _, q := range followedQuivers {
		followedSet[q.Namespace] = struct{}{}
	}
	var result []models.QuiverListDTO
	for _, ns := range cached {
		if _, ok := followedSet[ns]; ok {
			continue
		}
		manifest, _, getErr := u.repo.Get(ctx, ns)
		if getErr != nil {
			continue
		}
		result = append(result, models.QuiverListDTO{
			Namespace:   ns,
			Name:        manifest.Name,
			Description: manifest.Description,
			Tags:        manifest.Tags,
			ArrowCount:  len(manifest.Arrows),
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
	manifest, err := u.manifold.ParseQuiver(data, ns)
	if err != nil {
		return fmt.Errorf("seed quiver: %w", err)
	}
	if _, err := u.vault.PutQuiver(ctx, ns, manifest, nil); err != nil {
		return fmt.Errorf("seed quiver: %w", err)
	}
	return nil
}

func (u *quiverUsecase) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) ([]byte, error) {
	manifest, _, err := u.repo.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get quiver manifest: %w", err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("get quiver manifest: marshal: %w", err)
	}
	return data, nil
}

func (u *quiverUsecase) ValidateManifest(
	_ context.Context,
	data []byte,
) (*models.ValidationResult, error) {
	_, err := u.manifold.ParseQuiver(data, "validation.dummy/quiver")
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

func (u *quiverUsecase) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.Follow(ctx, ns)
}

func (u *quiverUsecase) Update(
	_ context.Context,
	_ domain.Namespace,
) error {
	return nil
}

func (u *quiverUsecase) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.Unfollow(ctx, ns)
}
