package commands_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func buildAsynx(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func testNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@v1.0.0")
}

func seedArrow(
	t *testing.T,
	ax asynx.Asynx[domain.Arrow],
	ns domain.Namespace,
	userInstalled bool,
) {
	t.Helper()
	cmd := commands.AddArrow{
		Namespace:     ns,
		ArrowMeta:     domain.ArrowMeta{Name: "Test Arrow"},
		DirectInstall: userInstalled,
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)
}

// ─── AddArrow ────────────────────────────────────────────────────────────────

func TestAddArrow_Success(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	cmd := commands.AddArrow{
		Namespace:     ns,
		ArrowMeta:     domain.ArrowMeta{Name: "Test Arrow", Version: "v1.0.0"},
		DirectInstall: true,
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ns, got.Namespace)
	assert.Equal(t, "Test Arrow", got.Name)
	assert.True(t, got.UserInstalled)
}

func TestAddArrow_OnExisting_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	cmd := commands.AddArrow{Namespace: ns}
	_, err := ax.Send(context.Background(), cmd)
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestAddArrow_DirectInstall_False(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	cmd := commands.AddArrow{
		Namespace:     ns,
		DirectInstall: false,
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, got.UserInstalled)
}

func TestAddArrow_InstalledConstraint(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	cmd := commands.AddArrow{
		Namespace:           ns,
		InstalledConstraint: "^v1",
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "^v1", got.InstalledConstraint)
}

// ─── MarkInstalled ───────────────────────────────────────────────────────────

func TestMarkInstalled_WithoutPriorAdd_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	cmd := commands.MarkInstalled{
		Namespace:    ns,
		InstalledRef: "v1.0.0",
		InstalledAt:  time.Now(),
	}
	_, err := ax.Send(context.Background(), cmd)
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestMarkInstalled_AfterAdd_SetsFields(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	now := time.Now().UTC().Truncate(time.Second)
	cmd := commands.MarkInstalled{
		Namespace:    ns,
		InstalledRef: "v1.0.0",
		InstalledAt:  now,
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", got.InstalledRef)
	assert.Equal(t, now, got.InstalledAt.UTC().Truncate(time.Second))
}

// Every resolution path settles a ref before the arrow reaches the catalog, so
// the ref the install is stamped with is the one the namespace already carries.
func TestMarkInstalled_RecordsTheNamespaceRef(t *testing.T) {
	testCases := []struct {
		name    string
		ns      domain.Namespace
		ref     string
		wantRef string
	}{
		{
			name:    "tag",
			ns:      domain.Namespace("github.com/user/repo@v1.2.3"),
			ref:     "v1.2.3",
			wantRef: "v1.2.3",
		},
		{
			name:    "default branch",
			ns:      domain.Namespace("github.com/user/repo@master"),
			ref:     "master",
			wantRef: "master",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := buildAsynx(t)
			seedArrow(t, ax, tc.ns, false)

			_, err := ax.Send(context.Background(), commands.MarkInstalled{
				Namespace:    tc.ns,
				InstalledRef: tc.ref,
				InstalledAt:  time.Now().UTC(),
			})
			require.NoError(t, err)

			got, err := ax.Get(context.Background(), tc.ns.String())
			require.NoError(t, err)
			assert.Equal(t, tc.wantRef, got.InstalledRef)
		})
	}
}

// A re-install at the same ref must overwrite the stamp rather than accumulate
// state, so replaying the command twice is indistinguishable from once.
func TestMarkInstalled_Reapplied_OverwritesTheStamp(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	first := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	_, err := ax.Send(context.Background(), commands.MarkInstalled{
		Namespace:    ns,
		InstalledRef: "v0.9.0",
		InstalledAt:  first,
	})
	require.NoError(t, err)

	second := time.Now().UTC().Truncate(time.Second)
	_, err = ax.Send(context.Background(), commands.MarkInstalled{
		Namespace:    ns,
		InstalledRef: "v1.0.0",
		InstalledAt:  second,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", got.InstalledRef)
	assert.Equal(t, second, got.InstalledAt.UTC().Truncate(time.Second))
}

// ─── SetUserInstalled ────────────────────────────────────────────────────────

func TestSetUserInstalled_WithoutPriorAdd_Fails(t *testing.T) {
	ax := buildAsynx(t)

	cmd := commands.SetUserInstalled{Namespace: testNs()}
	_, err := ax.Send(context.Background(), cmd)
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestSetUserInstalled_SetsUserInstalled(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	cmd := commands.SetUserInstalled{Namespace: ns}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.UserInstalled)
}

// ─── UpdateArrowManifest ──────────────────────────────────────────────────────

func TestUpdateArrowManifest_WithoutPriorAdd_Fails(t *testing.T) {
	ax := buildAsynx(t)

	cmd := commands.UpdateArrowManifest{
		Namespace: testNs(),
		ArrowMeta: domain.ArrowMeta{Name: "New Name"},
	}
	_, err := ax.Send(context.Background(), cmd)
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestUpdateArrowManifest_UpdatesFields(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	cmd := commands.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Updated Name", Version: "v2.0.0"},
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", got.Name)
	assert.Equal(t, "v2.0.0", got.Version)
}

// ─── UpgradeArrow ─────────────────────────────────────────────────────────────

func TestUpgradeArrow_OnExisting_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	cmd := commands.UpgradeArrow{Namespace: ns}
	_, err := ax.Send(context.Background(), cmd)
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestUpgradeArrow_Success_SetsFields(t *testing.T) {
	ax := buildAsynx(t)
	newNs := domain.Namespace("github.com/user/repo@v2.0.0")
	oldNs := testNs()

	cmd := commands.UpgradeArrow{
		Namespace:           newNs,
		OldNamespace:        oldNs,
		ArrowMeta:           domain.ArrowMeta{Name: "Test Arrow", Version: "v2.0.0"},
		InstalledConstraint: "^v2",
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), newNs.String())
	require.NoError(t, err)
	assert.Equal(t, newNs, got.Namespace)
	assert.Equal(t, "v2.0.0", got.Version)
	assert.Equal(t, "^v2", got.InstalledConstraint)
	assert.Equal(t, oldNs, got.UpgradedFromNs)
	assert.False(t, got.UserInstalled)
}

// ─── Validate helpers ─────────────────────────────────────────────────────────

func isValidationErr(err error) bool {
	return errors.Is(err, asynxModels.ErrValidation) ||
		errors.Is(err, asynxModels.ErrPipelineFailed)
}
