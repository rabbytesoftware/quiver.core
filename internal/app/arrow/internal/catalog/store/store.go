package store

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

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
	ListVersions(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Arrow, error)
}

type arrowRow struct {
	Namespace     string `gorm:"primaryKey"`
	BareNamespace string `gorm:"index;not null"`
	Manifest      string `gorm:"not null"`
}

func (arrowRow) TableName() string { return "arrows" }

type arrowCatalog struct {
	inner adapterstore.Store[arrowRow, string]
	db    *gorm.DB
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

// NewArrowCatalogFromDB creates an ArrowCatalog backed by an already-open *gorm.DB.
// The arrows table is auto-migrated into the provided DB.
func NewArrowCatalogFromDB(
	db *gorm.DB,
) (ArrowCatalog, error) {
	inner, err := sqlite.NewFromDB[arrowRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("arrow catalog: %w", err)
	}
	return &arrowCatalog{inner: inner, db: db}, nil
}

func (c *arrowCatalog) Save(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	manifest, _ := json.Marshal(arrow)

	return c.inner.Save(ctx, arrowRow{
		Namespace:     arrow.Namespace.String(),
		BareNamespace: arrow.Namespace.BareNamespace().String(),
		Manifest:      string(manifest),
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

	return unmarshalArrowRow(*row)
}

func (c *arrowCatalog) List(
	ctx context.Context,
) ([]domain.Arrow, error) {
	rows, err := c.inner.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	return unmarshalArrowRows(rows)
}

func (c *arrowCatalog) ListVersions(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Arrow, error) {
	if c.db == nil {
		rows, err := c.inner.FindAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("list versions: %w", err)
		}
		bare := ns.BareNamespace().String()
		var filtered []arrowRow
		for _, row := range rows {
			if row.BareNamespace == bare {
				filtered = append(filtered, row)
			}
		}
		return unmarshalArrowRows(filtered)
	}

	bare := ns.BareNamespace().String()
	var rows []arrowRow
	if err := c.db.WithContext(ctx).
		Where("bare_namespace = ?", bare).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return unmarshalArrowRows(rows)
}

func unmarshalArrowRow(
	row arrowRow,
) (*domain.Arrow, error) {
	var arrow domain.Arrow
	if err := json.Unmarshal([]byte(row.Manifest), &arrow); err != nil {
		return nil, err
	}
	return &arrow, nil
}

func unmarshalArrowRows(
	rows []arrowRow,
) ([]domain.Arrow, error) {
	arrows := make([]domain.Arrow, 0, len(rows))
	for _, row := range rows {
		a, err := unmarshalArrowRow(row)
		if err != nil {
			return nil, err
		}
		arrows = append(arrows, *a)
	}
	return arrows, nil
}
