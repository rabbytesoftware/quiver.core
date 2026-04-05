package sqlite

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/char2cs/asynx/models"
)

type eventEntry struct {
	AggregateID string `db:"aggregate_id"`
	Version     int64  `db:"version"`
	Data        []byte `db:"data"`
}

//go:embed migrations/*.sql
var migrations embed.FS

type eventStore struct {
	db *sqlx.DB
}

// NewEventStore returns a SQLite-backed asynx event store.
// Creates or opens a SQLite database at the given path for event persistence.
func NewEventStore(path string) (models.Store, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("eventstore: open db: %w", err)
	}

	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}

	migrationsFS, err := fs.Sub(migrations, "migrations")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("eventstore: migrate: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db.DB, migrationsFS)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("eventstore: migrate: %w", err)
	}

	if _, err := provider.Up(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("eventstore: migrate: %w", err)
	}

	return &eventStore{db: db}, nil
}

func (s *eventStore) Append(
	ctx context.Context,
	aggregateID string,
	version int64,
	data []byte,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO events (aggregate_id, version, data) VALUES (?, ?, ?)`,
		aggregateID, version, data,
	)
	if err != nil {
		return fmt.Errorf("%w: version conflict (%s, v%d)", models.ErrPipelineFailed, aggregateID, version)
	}

	return nil
}

func (s *eventStore) ReadFrom(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var entries []eventEntry
	err := s.db.SelectContext(
		ctx,
		&entries,
		`SELECT aggregate_id, version, data FROM events
		 WHERE aggregate_id = ? AND version >= ?
		 ORDER BY version ASC`,
		aggregateID, fromVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("eventstore: read from: %w", err)
	}

	result := make([][]byte, len(entries))
	for i, e := range entries {
		result[i] = e.Data
	}
	return result, nil
}

func (s *eventStore) ReadRange(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	count int64,
) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var entries []eventEntry
	err := s.db.SelectContext(
		ctx,
		&entries,
		`SELECT aggregate_id, version, data FROM events
		 WHERE aggregate_id = ? AND version >= ?
		 ORDER BY version ASC LIMIT ?`,
		aggregateID, fromVersion, count,
	)
	if err != nil {
		return nil, fmt.Errorf("eventstore: read range: %w", err)
	}

	result := make([][]byte, len(entries))
	for i, e := range entries {
		result[i] = e.Data
	}
	return result, nil
}

func (s *eventStore) Count(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	var count int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM events WHERE aggregate_id = ? AND version >= ?`,
		aggregateID, fromVersion,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("eventstore: count: %w", err)
	}

	return count, nil
}
