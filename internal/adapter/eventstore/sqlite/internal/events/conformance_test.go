package events

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/char2cs/asynx/store/storetest"
	"github.com/stretchr/testify/require"
)

func TestEventStore_Conformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) models.Store {
		s, err := New(filepath.Join(t.TempDir(), "events.db"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })
		return s
	})
}

func appendEvent(s *Store, ctx context.Context, streamID string, version int64, data []byte) error {
	return s.Atomically(ctx, func(tx models.Tx) error {
		return tx.Append(ctx, streamID, version, data)
	})
}

func deleteStream(s *Store, ctx context.Context, streamID string) error {
	return s.Atomically(ctx, func(tx models.Tx) error {
		return tx.DeleteStream(ctx, streamID)
	})
}
