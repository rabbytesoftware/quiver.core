package selfarrow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/selfarrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestEnsureRegistered_AddsWhenAbsent(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
	}
	var addedNS domain.Namespace
	m.AddFn = func(_ context.Context, ns domain.Namespace) error {
		addedNS = ns
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/rabbytesoftware/quiver.core@stable-25.9.2"), addedNS)
}

func TestEnsureRegistered_SkipsWhenPresent(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	m.AddFn = func(context.Context, domain.Namespace) error {
		t.Fatal("Add must not be called when the self-arrow already exists")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

	require.NoError(t, err)
}

func TestEnsureRegistered_DevBuildSkipsRegistration(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) {
			t.Fatal("Exists must not be called for an unstamped dev build")
			return false, nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "dev")

	require.NoError(t, err)
}

func TestEnsureRegistered_EmptyVersionSkipsRegistration(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) {
			t.Fatal("Exists must not be called for an empty, unstamped version")
			return false, nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "")

	require.NoError(t, err)
}

func TestEnsureRegistered_ExistsFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("catalog unavailable")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) {
			return false, sentinel
		},
		AddFn: func(context.Context, domain.Namespace) error {
			t.Fatal("Add must not be called when Exists fails")
			return nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestEnsureRegistered_AddFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("add failed")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		AddFn: func(context.Context, domain.Namespace) error {
			return sentinel
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}
