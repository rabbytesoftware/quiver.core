package store

import (
	"context"
	"fmt"
	"time"

	adapterstore "github.com/rabbytesoftware/quiver/internal/adapter/store"
	"github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type QuiverStore interface {
	Save(
		ctx context.Context,
		quiver domain.Quiver,
	) error
	Delete(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Quiver, error)
	List(
		ctx context.Context,
	) ([]domain.Quiver, error)
}

type quiverRow struct {
	Namespace  string `gorm:"primaryKey"`
	FollowedAt int64  `gorm:"not null"`
}

func (quiverRow) TableName() string { return "quivers" }

type quiverStore struct {
	inner adapterstore.Store[quiverRow, string]
}

func New(
	path string,
) (QuiverStore, error) {
	inner, err := sqlite.New[quiverRow, string](path)
	if err != nil {
		return nil, fmt.Errorf("quiver store: %w", err)
	}
	return &quiverStore{inner: inner}, nil
}

func (s *quiverStore) Save(
	ctx context.Context,
	quiver domain.Quiver,
) error {
	return s.inner.Save(ctx, quiverRow{
		Namespace:  quiver.Namespace.String(),
		FollowedAt: quiver.FollowedAt.Unix(),
	})
}

func (s *quiverStore) Delete(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return s.inner.Delete(ctx, ns.String())
}

func (s *quiverStore) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Quiver, error) {
	row, err := s.inner.FindByKey(ctx, ns.String())
	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, nil
	}

	return &domain.Quiver{
		Namespace:  domain.Namespace(row.Namespace),
		FollowedAt: time.Unix(row.FollowedAt, 0),
	}, nil
}

func (s *quiverStore) List(
	ctx context.Context,
) ([]domain.Quiver, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	quivers := make([]domain.Quiver, 0, len(rows))
	for _, row := range rows {
		quivers = append(quivers, domain.Quiver{
			Namespace:  domain.Namespace(row.Namespace),
			FollowedAt: time.Unix(row.FollowedAt, 0),
		})
	}
	return quivers, nil
}
