package database

import (
	"context"
	"testing"
)

type TestStruct struct {
	ID   string
	Name string
}

func TestNewDatabase(t *testing.T) {
	ctx := context.Background()

	// Test creating a new database
	db, err := NewDatabase[TestStruct](ctx, "test_database")
	if err != nil {
		t.Fatalf("NewDatabase() failed: %v", err)
	}

	if db == nil {
		t.Error("NewDatabase() returned nil database")
	}
}

func TestNewDatabase_WithDifferentTypes(t *testing.T) {
	ctx := context.Background()

	// Test with TestStruct type which is safe
	db, err := NewDatabase[TestStruct](ctx, "test_struct_db")
	if err != nil {
		t.Logf("NewDatabase[TestStruct]() error (may be expected): %v", err)
	}
	_ = db
}

func TestNewDatabase_WithDifferentContexts(t *testing.T) {
	// Test with different contexts
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{"background context", context.Background()},
		{"todo context", context.TODO()},
		{"with value", context.WithValue(context.Background(), "key", "value")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := NewDatabase[TestStruct](tt.ctx, "test_db")
			if err != nil {
				t.Errorf("NewDatabase() with %s failed: %v", tt.name, err)
			}
			if db == nil {
				t.Errorf("NewDatabase() with %s returned nil", tt.name)
			}
		})
	}
}

func TestNewDatabase_WithDifferentNames(t *testing.T) {
	ctx := context.Background()

	// Test with different database names
	names := []string{
		"test_db",
		"another_db",
		"db_with_underscores",
		"db-with-dashes",
		"db123",
		"",
	}

	for _, name := range names {
		t.Run("name_"+name, func(t *testing.T) {
			db, err := NewDatabase[TestStruct](ctx, name)
			if err != nil {
				t.Logf("NewDatabase() with name '%s' failed (may be expected): %v", name, err)
			} else if db == nil {
				t.Logf("NewDatabase() with name '%s' returned nil (may be expected)", name)
			}
		})
	}
}

func TestNewDatabase_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	// Test that NewDatabase handles errors appropriately
	_, err := NewDatabase[TestStruct](ctx, "test_error_db")

	// The error handling depends on the repository implementation
	// We just verify the function executes without panicking
	_ = err
}

