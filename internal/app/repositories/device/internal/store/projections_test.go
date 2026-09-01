package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	devicestore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/device/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func TestProjector_Apply_WritesToStore(t *testing.T) {
	st, err := devicestore.New(newTestDB(t))
	require.NoError(t, err)

	p := devicestore.NewProjector(st)
	now := time.Now()

	err = p.Apply(context.Background(), auth.Device{
		ID: "dev-1", Label: "laptop", TokenHash: "hash-1",
		State: auth.DeviceStateActive, PairedAt: now, LastSeenAt: now,
	})
	require.NoError(t, err)

	got, err := st.Get(context.Background(), "dev-1")
	require.NoError(t, err)
	assert.Equal(t, "laptop", got.Label)
}
