package bound

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/bind/internal/binder"
)

type agg struct{ N int }

func TestBound_ForwardsToItsBinderAndHostsDB(t *testing.T) {
	router := binder.New[agg]()
	b := New[agg](nil, router, nil)

	require.NoError(t, b.Durable("thing.*", func(context.Context, asynxModels.Event[agg]) error { return nil }))
	require.NoError(t, b.Sync("thing.*", func(context.Context, asynxModels.Tx, asynxModels.Event[agg]) error { return nil }))
	assert.Nil(t, b.Host())

	done := make(chan error, 1)
	go func() {
		done <- router.DispatchDurable(context.Background(), asynxModels.Event[agg]{EventName: "thing.x"})
	}()
	b.Ready()
	require.NoError(t, <-done)
}
