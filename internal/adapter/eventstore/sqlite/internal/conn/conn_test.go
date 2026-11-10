package conn

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOpen(t *testing.T) {
	testCases := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr bool
	}{
		{name: "creates the file", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "a.db") }},
		{name: "missing directory fails", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "no", "a.db") }, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := Open(tc.path(t))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, db.Exec("CREATE TABLE t (id INTEGER)").Error)
			Close(db)
		})
	}
}

func TestClose_AlreadyClosedIsHarmless(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "a.db"))
	require.NoError(t, err)

	Close(db)
	assert.NotPanics(t, func() { Close(db) })
}

func TestClose_WithoutAPool_IsHarmless(t *testing.T) {
	db := &gorm.DB{Config: &gorm.Config{}, Statement: &gorm.Statement{}}

	assert.NotPanics(t, func() { Close(db) })
}
