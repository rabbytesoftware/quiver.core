package recommendationinternal_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	recommendationinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal"
)

func TestHooks_OnRefreshed_NilCallback_ReturnsError(t *testing.T) {
	var hooks recommendationinternal.Hooks

	require.Error(t, hooks.OnRefreshed(nil))
}

func TestHooks_Fire_RunsEveryCallbackInRegistrationOrder(t *testing.T) {
	var hooks recommendationinternal.Hooks
	var order []int
	require.NoError(t, hooks.OnRefreshed(func(context.Context) { order = append(order, 1) }))
	require.NoError(t, hooks.OnRefreshed(func(context.Context) { order = append(order, 2) }))

	hooks.Fire(context.Background())

	assert.Equal(t, []int{1, 2}, order)
}

func TestHooks_Fire_WithNoCallbacks_IsANoop(t *testing.T) {
	var hooks recommendationinternal.Hooks

	assert.NotPanics(t, func() { hooks.Fire(context.Background()) })
}
