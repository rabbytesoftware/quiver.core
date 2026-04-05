package store

import (
	"embed"
	"encoding/json"
	"fmt"

	adapterstore "github.com/rabbytesoftware/quiver/internal/adapter/store"
	"github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

type ArrowCatalog interface {
	Save(arrow domain.Arrow) error
	Delete(ns domain.Namespace) error
	Get(ns domain.Namespace) (*domain.Arrow, error)
	List() ([]domain.Arrow, error)
}

type arrowRow struct {
	Namespace string `db:"namespace"`
	Manifest  string `db:"manifest"`
	Removed   bool   `db:"removed"`
}

type arrowCatalog struct {
	inner adapterstore.Store[arrowRow, string]
}

func NewArrowCatalog(
	path string,
) (ArrowCatalog, error) {
	db, err := sqlite.Open(path, migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("arrow catalog: %w", err)
	}
	return &arrowCatalog{inner: sqlite.New[arrowRow, string](db, "arrows", "namespace")}, nil
}

func (ac *arrowCatalog) Save(
	arrow domain.Arrow,
) error {
	manifestJSON, err := json.Marshal(arrow.Manifest)
	if err != nil {
		return err
	}

	row := arrowRow{
		Namespace: arrow.Namespace.String(),
		Manifest:  string(manifestJSON),
		Removed:   arrow.Removed,
	}

	return ac.inner.Save(row)
}

func (ac *arrowCatalog) Delete(
	ns domain.Namespace,
) error {
	return ac.inner.Delete(ns.String())
}

func (ac *arrowCatalog) Get(
	ns domain.Namespace,
) (*domain.Arrow, error) {
	row, err := ac.inner.FindByKey(ns.String())
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
		Removed:   row.Removed,
	}, nil
}

func (ac *arrowCatalog) List() ([]domain.Arrow, error) {
	rows, err := ac.inner.FindAll()
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
			Removed:   row.Removed,
		})
	}

	return arrows, nil
}
