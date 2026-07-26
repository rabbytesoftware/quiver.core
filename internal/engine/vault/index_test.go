package vault

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenIndex_CreatesSchema(t *testing.T) {
	idx, err := openIndex(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	require.NotNil(t, idx)

	var n int64
	require.NoError(t, idx.db.Raw(
		`SELECT count(*) FROM sqlite_master WHERE name = 'vault_arrows_fts'`,
	).Scan(&n).Error)
	require.Equal(t, int64(1), n, "FTS5 virtual table must exist")
}

func TestOpenIndex_IsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	first, err := openIndex(path)
	require.NoError(t, err)
	require.NoError(t, first.close())

	second, err := openIndex(path)
	require.NoError(t, err)
	require.NoError(t, second.close())
}
