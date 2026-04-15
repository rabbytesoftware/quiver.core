package store

import (
	"context"
	"encoding/json"
	"fmt"

	adapterstore "github.com/rabbytesoftware/quiver/internal/adapter/store"
	"github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type QuiverCatalog interface {
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
	Namespace string `gorm:"primaryKey"`
	Manifest  string `gorm:"not null"`
}

func (quiverRow) TableName() string { return "quivers" }

type quiverCatalog struct {
	inner adapterstore.Store[quiverRow, string]
}

func NewQuiverCatalog(
	path string,
) (QuiverCatalog, error) {
	inner, err := sqlite.New[quiverRow, string](path)
	if err != nil {
		return nil, fmt.Errorf("quiver catalog: %w", err)
	}
	return &quiverCatalog{inner: inner}, nil
}

func (c *quiverCatalog) Save(
	ctx context.Context,
	quiver domain.Quiver,
) error {
	manifest, _ := json.Marshal(quiver.Manifest)

	return c.inner.Save(ctx, quiverRow{
		Namespace: quiver.Namespace.String(),
		Manifest:  string(manifest),
	})
}

func (c *quiverCatalog) Delete(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return c.inner.Delete(ctx, ns.String())
}

func (c *quiverCatalog) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Quiver, error) {
	row, err := c.inner.FindByKey(ctx, ns.String())
	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, nil
	}

	var manifest domain.QuiverManifest
	if err := json.Unmarshal([]byte(row.Manifest), &manifest); err != nil {
		return nil, err
	}

	return &domain.Quiver{
		Namespace: domain.Namespace(row.Namespace),
		Manifest:  manifest,
	}, nil
}

func (c *quiverCatalog) List(
	ctx context.Context,
) ([]domain.Quiver, error) {
	rows, err := c.inner.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	quivers := make([]domain.Quiver, 0, len(rows))
	for _, row := range rows {
		var manifest domain.QuiverManifest
		if err := json.Unmarshal([]byte(row.Manifest), &manifest); err != nil {
			return nil, err
		}
		quivers = append(quivers, domain.Quiver{
			Namespace: domain.Namespace(row.Namespace),
			Manifest:  manifest,
		})
	}
	return quivers, nil
}
