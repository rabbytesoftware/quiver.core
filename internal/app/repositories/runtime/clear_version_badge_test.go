package runtime_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestRuntime_ClearVersionBadge_OutdatedWithoutDepSync_BecomesReady(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateOutdated)
	repo := newRepoWithAssembler(t, ax, successAssembler())

	require.NoError(t, repo.ClearVersionBadge(context.Background(), ns))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestRuntime_ClearVersionBadge_PendingDepSync_StaysOutdated(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)
	repo := newRepoWithAssembler(t, ax, successAssembler())
	require.NoError(t, repo.MarkOutdated(context.Background(), ns, []domain.Namespace{"a/b/c@v1"}, nil))
	ax.WaitPublish()

	require.NoError(t, repo.ClearVersionBadge(context.Background(), ns))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
	require.NotNil(t, got.PendingDepSync)
}

func TestRuntime_ClearVersionBadge_NoOpStates(t *testing.T) {
	testCases := []struct {
		name  string
		seed  bool
		state domain.ArrowState
	}{
		{name: "ready stays ready", seed: true, state: domain.ArrowStateReady},
		{name: "running is not a badge", seed: true, state: domain.ArrowStateRunning},
		{name: "no aggregate grows none", seed: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := newTestAsynxRuntime(t)
			ns := testNs()
			if tc.seed {
				seedState(t, ax, ns, tc.state)
			}
			cleared := countTopic(t, ax, "runtime.outdated_cleared.*")
			repo := newRepoWithAssembler(t, ax, successAssembler())

			require.NoError(t, repo.ClearVersionBadge(context.Background(), ns))
			ax.WaitPublish()

			assert.Zero(t, cleared.Load())
			exists, err := ax.Exists(context.Background(), ns.String())
			require.NoError(t, err)
			assert.Equal(t, tc.seed, exists)
		})
	}
}

func TestRuntime_ClearVersionBadge_ClosedStore_ReturnsError(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateOutdated)
	repo := newRepoWithAssembler(t, ax, successAssembler())
	require.NoError(t, ax.Shutdown(context.Background()))

	assert.Error(t, repo.ClearVersionBadge(context.Background(), ns))
}
