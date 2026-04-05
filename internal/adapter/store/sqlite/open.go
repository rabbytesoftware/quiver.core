package sqlite

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

// Open opens a SQLite database at path and runs all pending goose migrations
// from the provided embedded FS subdirectory.
// For :memory: paths, pins to a single connection to prevent each pool
// connection from getting its own empty in-memory database.
func Open(
	path string,
	migrations fs.FS,
	dir string,
) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}

	migrationsFS, err := fs.Sub(migrations, dir)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migrations fs: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db.DB, migrationsFS)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite goose provider: %w", err)
	}

	if _, err := provider.Up(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}

	return db, nil
}
