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
		vm ArrowViewModel,
	) error
	Delete(
		ctx context.Context,
		bareNs domain.Namespace,
	) error
	Get(
		ctx context.Context,
		bareNs domain.Namespace,
	) (*ArrowViewModel, error)
	List(
		ctx context.Context,
	) ([]ArrowViewModel, error)
}

type arrowRow struct {
	Namespace string `gorm:"primaryKey"`
	ViewModel string `gorm:"not null"`
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
	vm ArrowViewModel,
) error {
	data, err := json.Marshal(vm)
	if err != nil {
		return fmt.Errorf("arrow catalog save: marshal: %w", err)
	}
	return c.inner.Save(ctx, arrowRow{
		Namespace: vm.Namespace.String(),
		ViewModel: string(data),
	})
}

func (c *arrowCatalog) Delete(
	ctx context.Context,
	bareNs domain.Namespace,
) error {
	return c.inner.Delete(ctx, bareNs.String())
}

func (c *arrowCatalog) Get(
	ctx context.Context,
	bareNs domain.Namespace,
) (*ArrowViewModel, error) {
	row, err := c.inner.FindByKey(ctx, bareNs.String())
	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, nil
	}

	return unmarshalViewModelRow(*row)
}

func (c *arrowCatalog) List(
	ctx context.Context,
) ([]ArrowViewModel, error) {
	rows, err := c.inner.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	return unmarshalViewModelRows(rows)
}

func unmarshalViewModelRow(
	row arrowRow,
) (*ArrowViewModel, error) {
	var vm ArrowViewModel
	if err := json.Unmarshal([]byte(row.ViewModel), &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func unmarshalViewModelRows(
	rows []arrowRow,
) ([]ArrowViewModel, error) {
	vms := make([]ArrowViewModel, 0, len(rows))
	for _, row := range rows {
		vm, err := unmarshalViewModelRow(row)
		if err != nil {
			return nil, err
		}
		vms = append(vms, *vm)
	}
	return vms, nil
}
