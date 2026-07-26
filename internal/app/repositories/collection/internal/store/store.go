package store

import (
	"context"
	"encoding/json"
	"fmt"

	adapterstore "github.com/rabbytesoftware/quiver.core/internal/adapter/store"
	"github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type QuiverStore interface {
	Save(
		ctx context.Context,
		coll domain.Collection,
	) error
	Delete(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Collection, error)
	List(
		ctx context.Context,
	) ([]domain.Collection, error)
	// Close releases the collections database. It must run after the collection
	// aggregate has drained, otherwise a still-running projection writes to a
	// closed handle. A drain that ran out of budget is exactly that case: it
	// returns without its projections having finished, and their remaining writes
	// fail here rather than being waited for.
	Close() error
}

type collectionRow struct {
	Namespace string `gorm:"primaryKey"`
	Data      string `gorm:"not null"`
}

func (collectionRow) TableName() string { return "collections" }

type collectionStore struct {
	inner adapterstore.Store[collectionRow, string]
}

func New(
	path string,
) (QuiverStore, error) {
	inner, err := sqlite.New[collectionRow, string](path)
	if err != nil {
		return nil, fmt.Errorf("collection store: %w", err)
	}
	return &collectionStore{inner: inner}, nil
}

func (s *collectionStore) Save(
	ctx context.Context,
	coll domain.Collection,
) error {
	data, err := json.Marshal(coll)
	if err != nil {
		return fmt.Errorf("collection store save: marshal: %w", err)
	}
	return s.inner.Save(ctx, collectionRow{
		Namespace: coll.Namespace.String(),
		Data:      string(data),
	})
}

func (s *collectionStore) Delete(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return s.inner.Delete(ctx, ns.String())
}

func (s *collectionStore) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Collection, error) {
	row, err := s.inner.FindByKey(ctx, ns.String())
	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, nil
	}

	var q domain.Collection
	if err := json.Unmarshal([]byte(row.Data), &q); err != nil {
		return nil, fmt.Errorf("collection store get: unmarshal: %w", err)
	}
	return &q, nil
}

func (s *collectionStore) Close() error {
	if err := s.inner.Close(); err != nil {
		return fmt.Errorf("collection store close: %w", err)
	}
	return nil
}

func (s *collectionStore) List(
	ctx context.Context,
) ([]domain.Collection, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	colls := make([]domain.Collection, 0, len(rows))
	for _, row := range rows {
		var q domain.Collection
		if err := json.Unmarshal([]byte(row.Data), &q); err != nil {
			return nil, fmt.Errorf("collection store list: unmarshal: %w", err)
		}
		colls = append(colls, q)
	}
	return colls, nil
}
