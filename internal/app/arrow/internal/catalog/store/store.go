package store

import (
	"context"
	"encoding/json"
	"fmt"

	adapterstore "github.com/rabbytesoftware/quiver/internal/adapter/store"
	"github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type ArrowCatalog interface {
	Save(
		ctx context.Context,
		arrow domain.Arrow,
	) error
	Delete(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	List(
		ctx context.Context,
	) ([]domain.Arrow, error)
}

type arrowRow struct {
	Namespace string `gorm:"primaryKey"`
	Manifest  string `gorm:"not null"`
}

func (arrowRow) TableName() string { return "arrows" }

type arrowCatalog struct {
	inner adapterstore.Store[arrowRow, string]
}

func NewArrowCatalog(
	path string,
) (ArrowCatalog, error) {
	inner, err := sqlite.New[arrowRow, string](path)
	if err != nil {
		return nil, fmt.Errorf("arrow catalog: %w", err)
	}

	return &arrowCatalog{inner: inner}, nil
}

func (c *arrowCatalog) Save(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	manifest, _ := json.Marshal(arrow.Manifest)

	return c.inner.Save(ctx, arrowRow{
		Namespace: arrow.Namespace.String(),
		Manifest:  string(manifest),
	})
}

func (c *arrowCatalog) Delete(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return c.inner.Delete(ctx, ns.String())
}

func (c *arrowCatalog) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	row, err := c.inner.FindByKey(ctx, ns.String())
	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, nil
	}

	var manifest domain.ArrowManifest
	if err := json.Unmarshal([]byte(row.Manifest), &manifest); err != nil {
		return nil, err
	}

	return &domain.Arrow{
		Namespace: domain.Namespace(row.Namespace),
		Manifest:  manifest,
	}, nil
}

func (c *arrowCatalog) List(
	ctx context.Context,
) ([]domain.Arrow, error) {
	rows, err := c.inner.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	arrows := make([]domain.Arrow, 0, len(rows))
	for _, row := range rows {
		var manifest domain.ArrowManifest
		if err := json.Unmarshal([]byte(row.Manifest), &manifest); err != nil {
			return nil, err
		}

		arrows = append(arrows, domain.Arrow{
			Namespace: domain.Namespace(row.Namespace),
			Manifest:  manifest,
		})
	}

	return arrows, nil
}
