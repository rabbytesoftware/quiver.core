package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
	_ "modernc.org/sqlite"
)

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
	db *sqlx.DB
	mu sync.RWMutex
}

func NewQuiverCatalog() (QuiverCatalog, error) {
	return NewQuiverCatalogFromPath(metadata.GetQuiverHome() + "/quivers.db")
}

func NewQuiverCatalogFromPath(path string) (QuiverCatalog, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("quiver catalog: open: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS quivers (
			namespace TEXT PRIMARY KEY,
			manifest TEXT NOT NULL,
			removed INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("quiver catalog: schema: %w", err)
	}

	return &quiverCatalog{db: db}, nil
}

func (qc *quiverCatalog) Save(quiver domain.Quiver) error {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	manifestJSON, err := json.Marshal(quiver.Manifest)
	if err != nil {
		return err
	}

	row := quiverRow{
		Namespace: quiver.Namespace.String(),
		Manifest:  string(manifestJSON),
		Removed:   quiver.Removed,
	}

	_, err = qc.db.NamedExec(
		`INSERT OR REPLACE INTO quivers (namespace, manifest, removed) VALUES (:namespace, :manifest, :removed)`,
		row,
	)
	if err != nil {
		return fmt.Errorf("quiver catalog: save: %w", err)
	}

	return nil
}

func (qc *quiverCatalog) Delete(ns domain.Namespace) error {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	_, err := qc.db.Exec(`DELETE FROM quivers WHERE namespace = ?`, ns.String())
	if err != nil {
		return fmt.Errorf("quiver catalog: delete: %w", err)
	}

	return nil
}

func (qc *quiverCatalog) Get(ns domain.Namespace) (*domain.Quiver, error) {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	var row quiverRow
	err := qc.db.Get(&row, `SELECT namespace, manifest, removed FROM quivers WHERE namespace = ?`, ns.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	var manifest domain.QuiverManifest
	err = json.Unmarshal([]byte(row.Manifest), &manifest)
	if err != nil {
		return nil, err
	}

	return &domain.Quiver{
		Namespace: domain.Namespace(row.Namespace),
		Manifest:  manifest,
		Removed:   row.Removed,
	}, nil
}

func (qc *quiverCatalog) List() ([]domain.Quiver, error) {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	var rows []quiverRow
	err := qc.db.Select(&rows, `SELECT namespace, manifest, removed FROM quivers`)
	if err != nil {
		return nil, err
	}

	quivers := make([]domain.Quiver, 0, len(rows))
	for _, row := range rows {
		var manifest domain.QuiverManifest
		err := json.Unmarshal([]byte(row.Manifest), &manifest)
		if err != nil {
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
