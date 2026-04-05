// Package sqlite provides a generic SQLite-backed Store[T any, K comparable] implementation.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/rabbytesoftware/quiver/internal/adapter/store"
)

type sqliteStore[T any, K comparable] struct {
	db    *sqlx.DB
	table string
	pkCol string
}

// New returns a SQLite-backed Store[T, K] using the provided database connection.
// table is the SQL table name; pkCol is the db-tagged primary key column name.
func New[T any, K comparable](
	db *sqlx.DB,
	table string,
	pkCol string,
) store.Store[T, K] {
	return &sqliteStore[T, K]{
		db:    db,
		table: table,
		pkCol: pkCol,
	}
}

func (s *sqliteStore[T, K]) Save(
	item T,
) error {
	cols, err := getColumnNames(item)
	if err != nil {
		return err
	}

	placeholders := make([]string, len(cols))
	for i, col := range cols {
		placeholders[i] = ":" + col
	}

	query := fmt.Sprintf(
		`INSERT OR REPLACE INTO %s (%s) VALUES (%s)`,
		s.table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err = s.db.NamedExec(query, item)
	return err
}

func (s *sqliteStore[T, K]) Delete(
	id K,
) error {
	_, err := s.db.Exec(
		fmt.Sprintf(`DELETE FROM %s WHERE %s = ?`, s.table, s.pkCol),
		id,
	)
	return err
}

func (s *sqliteStore[T, K]) FindByKey(
	id K,
) (*T, error) {
	var item T
	err := s.db.Get(&item, fmt.Sprintf(`SELECT * FROM %s WHERE %s = ?`, s.table, s.pkCol), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (s *sqliteStore[T, K]) FindAll() ([]T, error) {
	var items []T
	err := s.db.Select(&items, fmt.Sprintf(`SELECT * FROM %s`, s.table))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []T{}, nil
		}
		return nil, err
	}
	return items, nil
}

func getColumnNames(item any) ([]string, error) {
	t := reflect.TypeOf(item)
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("t must be a struct, got %s", t.Kind())
	}

	var cols []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		dbTag := field.Tag.Get("db")
		if dbTag == "" || dbTag == "-" {
			continue
		}

		colName := strings.Split(dbTag, ",")[0]
		cols = append(cols, colName)
	}

	return cols, nil
}
