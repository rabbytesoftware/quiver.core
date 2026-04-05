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
	db *sqlx.DB
	mu sync.RWMutex
}

func NewArrowCatalog() (ArrowCatalog, error) {
	return NewArrowCatalogFromPath(metadata.GetQuiverHome() + "/arrows.db")
}

func NewArrowCatalogFromPath(path string) (ArrowCatalog, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("arrow catalog: open: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS arrows (
			namespace TEXT PRIMARY KEY,
			manifest TEXT NOT NULL,
			removed INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("arrow catalog: schema: %w", err)
	}

	return &arrowCatalog{db: db}, nil
}

func (ac *arrowCatalog) Save(arrow domain.Arrow) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	manifestJSON, err := json.Marshal(arrow.Manifest)
	if err != nil {
		return err
	}

	row := arrowRow{
		Namespace: arrow.Namespace.String(),
		Manifest:  string(manifestJSON),
		Removed:   arrow.Removed,
	}

	_, err = ac.db.NamedExec(
		`INSERT OR REPLACE INTO arrows (namespace, manifest, removed) VALUES (:namespace, :manifest, :removed)`,
		row,
	)
	if err != nil {
		return fmt.Errorf("arrow catalog: save: %w", err)
	}

	return nil
}

func (ac *arrowCatalog) Delete(ns domain.Namespace) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	_, err := ac.db.Exec(`DELETE FROM arrows WHERE namespace = ?`, ns.String())
	if err != nil {
		return fmt.Errorf("arrow catalog: delete: %w", err)
	}

	return nil
}

func (ac *arrowCatalog) Get(ns domain.Namespace) (*domain.Arrow, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var row arrowRow
	err := ac.db.Get(&row, `SELECT namespace, manifest, removed FROM arrows WHERE namespace = ?`, ns.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	var manifest domain.ArrowManifest
	err = json.Unmarshal([]byte(row.Manifest), &manifest)
	if err != nil {
		return nil, err
	}

	return &domain.Arrow{
		Namespace: domain.Namespace(row.Namespace),
		Manifest:  manifest,
		Removed:   row.Removed,
	}, nil
}

func (ac *arrowCatalog) List() ([]domain.Arrow, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var rows []arrowRow
	err := ac.db.Select(&rows, `SELECT namespace, manifest, removed FROM arrows`)
	if err != nil {
		return nil, err
	}

	arrows := make([]domain.Arrow, 0, len(rows))
	for _, row := range rows {
		var manifest domain.ArrowManifest
		err := json.Unmarshal([]byte(row.Manifest), &manifest)
		if err != nil {
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
