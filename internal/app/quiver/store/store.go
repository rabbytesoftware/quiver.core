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

type QuiverCatalog interface {
	Save(quiver domain.Quiver) error
	Delete(ns domain.Namespace) error
	Get(ns domain.Namespace) (*domain.Quiver, error)
	List() ([]domain.Quiver, error)
}

type quiverRow struct {
	Namespace string `db:"namespace"`
	Manifest  string `db:"manifest"`
	Removed   bool   `db:"removed"`
}

type quiverCatalog struct {
	inner adapterstore.Store[quiverRow, string]
}

func NewQuiverCatalog(path string) (QuiverCatalog, error) {
	db, err := sqlite.Open(path, migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("quiver catalog: %w", err)
	}
	return &quiverCatalog{inner: sqlite.New[quiverRow, string](db, "quivers", "namespace")}, nil
}

func (qc *quiverCatalog) Save(quiver domain.Quiver) error {
	manifestJSON, err := json.Marshal(quiver.Manifest)
	if err != nil {
		return err
	}

	row := quiverRow{
		Namespace: quiver.Namespace.String(),
		Manifest:  string(manifestJSON),
		Removed:   quiver.Removed,
	}

	return qc.inner.Save(row)
}

func (qc *quiverCatalog) Delete(ns domain.Namespace) error {
	return qc.inner.Delete(ns.String())
}

func (qc *quiverCatalog) Get(ns domain.Namespace) (*domain.Quiver, error) {
	row, err := qc.inner.FindByKey(ns.String())
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
		Removed:   row.Removed,
	}, nil
}

func (qc *quiverCatalog) List() ([]domain.Quiver, error) {
	rows, err := qc.inner.FindAll()
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
			Removed:   row.Removed,
		})
	}

	return quivers, nil
}
