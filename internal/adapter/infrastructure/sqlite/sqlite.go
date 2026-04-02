// Package sqlite provides a generic SQLite-backed Store[T any] implementation.
// Uses sqlx + reflection to auto-generate DDL from db: struct tags.
package sqlite

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/rabbytesoftware/quiver/internal/adapter/store"
)

type Store[T any] struct {
	db    *sqlx.DB
	table string
	pkCol string
}

// New opens (or creates) a SQLite-backed Store[T] at path.
// table is the SQL table name; pkCol is the db-tagged primary key column name.
// Reflects over T's db: tags once at startup to generate CREATE TABLE DDL.
// Go types map to SQL: int/int64 → INTEGER, string → TEXT, bool → INTEGER, float64 → REAL.
func New[T any](
	path string,
	table string,
	pkCol string,
) (store.Store[T], error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	// Reflect over T to generate DDL
	var zero T
	ddl, err := generateDDL(zero, table, pkCol)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: generate ddl: %w", err)
	}

	// Execute CREATE TABLE
	if _, err := db.Exec(ddl); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: create table: %w", err)
	}

	return &Store[T]{
		db:    db,
		table: table,
		pkCol: pkCol,
	}, nil
}

func (s *Store[T]) Save(
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

func (s *Store[T]) Delete(
	id int,
) error {
	_, err := s.db.Exec(
		fmt.Sprintf(`DELETE FROM %s WHERE %s = ?`, s.table, s.pkCol),
		id,
	)
	return err
}

func (s *Store[T]) FindByID(
	id int,
) (*T, error) {
	var item T
	err := s.db.Get(&item, fmt.Sprintf(`SELECT * FROM %s WHERE %s = ?`, s.table, s.pkCol), id)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (s *Store[T]) FindAll() ([]T, error) {
	var items []T
	err := s.db.Select(&items, fmt.Sprintf(`SELECT * FROM %s`, s.table))
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return []T{}, nil
		}
		return nil, err
	}
	return items, nil
}

// generateDDL reflects over T and generates a CREATE TABLE statement.
func generateDDL(zero any, table string, pkCol string) (string, error) {
	t := reflect.TypeOf(zero)
	if t.Kind() != reflect.Struct {
		return "", fmt.Errorf("t must be a struct, got %s", t.Kind())
	}

	var cols []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		dbTag := field.Tag.Get("db")
		if dbTag == "" || dbTag == "-" {
			continue
		}

		colName := strings.Split(dbTag, ",")[0]
		sqlType := goTypeToSQL(field.Type)

		if colName == pkCol {
			cols = append(cols, fmt.Sprintf(`%s %s PRIMARY KEY`, colName, sqlType))
		} else {
			cols = append(cols, fmt.Sprintf(`%s %s`, colName, sqlType))
		}
	}

	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (%s)`, table, strings.Join(cols, ", ")), nil
}

// getColumnNames extracts db-tagged column names from a struct instance.
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

// goTypeToSQL maps Go types to SQLite types.
func goTypeToSQL(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Int, reflect.Int32, reflect.Int64:
		return "INTEGER"
	case reflect.Float64:
		return "REAL"
	case reflect.Bool:
		return "INTEGER"
	case reflect.String:
		return "TEXT"
	default:
		return "TEXT"
	}
}
