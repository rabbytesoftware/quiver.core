package snapshots

import (
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/char2cs/asynx/store/storetest"
	"github.com/stretchr/testify/require"
)

func TestSnapshotStore_Conformance(t *testing.T) {
	storetest.RunSnapshots(t, func(t *testing.T) models.SnapshotStore {
		s, err := New(filepath.Join(t.TempDir(), "snapshots.db"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })
		return s
	})
}
