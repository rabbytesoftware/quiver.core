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
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithSnapshotStore(ss).
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
		ArrowMeta:     domain.ArrowMeta{Name: "Test Arrow"},
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

func TestAddArrow_SetsReadme(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	cmd := commands.AddArrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Test Arrow"},
		Readme:    "# Docs",
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "# Docs", got.Readme)
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
		Namespace:   ns,
		InstalledAt: time.Now(),
	}
	_, err := ax.Send(context.Background(), cmd)
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestMarkInstalled_AfterAdd_StampsInstalledAt(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	now := time.Now().UTC().Truncate(time.Second)
	cmd := commands.MarkInstalled{
		Namespace:   ns,
		InstalledAt: now,
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, now, got.InstalledAt.UTC().Truncate(time.Second))
	assert.Equal(t, "v1.0.0", got.Namespace.Ref(), "the stamped ref is the one the aggregate is keyed by")
}

// Which ref an install put on disk is answered by which aggregate carries the
// stamp: the command routes on the full namespace@ref, so a sibling ref of the
// same repo stays untouched. Both ref shapes a namespace can carry are covered,
// since the aggregate key is the whole string either way.
func TestMarkInstalled_StampsOnlyTheRefItNames(t *testing.T) {
	testCases := []struct {
		name      string
		installed domain.Namespace
		sibling   domain.Namespace
	}{
		{
			name:      "tag",
			installed: domain.Namespace("github.com/user/repo@v1.2.3"),
			sibling:   domain.Namespace("github.com/user/repo@v2.0.0"),
		},
		{
			name:      "default branch",
			installed: domain.Namespace("github.com/user/repo@master"),
			sibling:   domain.Namespace("github.com/user/repo@develop"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := buildAsynx(t)
			seedArrow(t, ax, tc.installed, false)
			seedArrow(t, ax, tc.sibling, false)

			_, err := ax.Send(context.Background(), commands.MarkInstalled{
				Namespace:   tc.installed,
				InstalledAt: time.Now().UTC(),
			})
			require.NoError(t, err)

			got, err := ax.Get(context.Background(), tc.installed.String())
			require.NoError(t, err)
			assert.False(t, got.InstalledAt.IsZero())
			assert.Equal(t, tc.installed.Ref(), got.Namespace.Ref())

			other, err := ax.Get(context.Background(), tc.sibling.String())
			require.NoError(t, err)
			assert.True(t, other.InstalledAt.IsZero(), "installing one ref must not stamp another")
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
		Namespace:   ns,
		InstalledAt: first,
	})
	require.NoError(t, err)

	second := time.Now().UTC().Truncate(time.Second)
	_, err = ax.Send(context.Background(), commands.MarkInstalled{
		Namespace:   ns,
		InstalledAt: second,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, second, got.InstalledAt.UTC().Truncate(time.Second))
}

// ─── MarkUninstalled ─────────────────────────────────────────────────────────

func TestMarkUninstalled_WithoutPriorAdd_Fails(t *testing.T) {
	ax := buildAsynx(t)

	_, err := ax.Send(context.Background(), commands.MarkUninstalled{Namespace: testNs()})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// The stamp an install left behind is the whole reason this command exists: an
// arrow whose _uninstall ran must stop reporting its ref is on disk.
func TestMarkUninstalled_AfterInstall_ClearsTheStamp(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	_, err := ax.Send(context.Background(), commands.MarkInstalled{
		Namespace:   ns,
		InstalledAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.MarkUninstalled{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.InstalledAt.IsZero())
	assert.Equal(t, ns, got.Namespace, "the catalog row keeps naming its ref after an uninstall")
}

// Uninstalling releases the disk, not the catalog entry. UserInstalled records
// the intent to keep the arrow around, and InstalledConstraint is written when
// the namespace is added, so an update can still resolve through it.
func TestMarkUninstalled_KeepsTheAddTimeFields(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	_, err := ax.Send(context.Background(), commands.AddArrow{
		Namespace:           ns,
		ArrowMeta:           domain.ArrowMeta{Name: "Test Arrow"},
		Variables:           []domain.Variable{{Name: "PORT"}},
		DirectInstall:       true,
		InstalledConstraint: "^v1",
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.MarkInstalled{
		Namespace:   ns,
		InstalledAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.MarkUninstalled{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.UserInstalled)
	assert.Equal(t, "^v1", got.InstalledConstraint)
	assert.Equal(t, ns, got.Namespace)
	assert.Equal(t, "Test Arrow", got.Name)
	assert.Len(t, got.Variables, 1, "the manifest survives an uninstall untouched")
}

// Uninstalling twice, or uninstalling something that was never installed, has
// to be indistinguishable from doing it once.
func TestMarkUninstalled_Reapplied_StaysCleared(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	for range 2 {
		_, err := ax.Send(context.Background(), commands.MarkUninstalled{Namespace: ns})
		require.NoError(t, err)
	}

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.InstalledAt.IsZero())
}

func TestMarkUninstalled_CommandContract(t *testing.T) {
	cmd := commands.MarkUninstalled{Namespace: testNs()}

	assert.Equal(t, testNs().String(), cmd.AggregateID())
	assert.Equal(t, "arrow.uninstalled."+testNs().String(), cmd.EventName())
	assert.True(t, cmd.ShouldSnapshot(), "an uninstall is a durable transition")
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
		ArrowMeta: domain.ArrowMeta{Name: "Updated Name"},
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", got.Name)
	assert.Equal(t, "v1.0.0", got.Namespace.Ref(), "a manifest update must not move the ref the aggregate is filed under")
}

func TestUpdateArrowManifest_UpdatesReadme(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	cmd := commands.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Updated Name"},
		Readme:    "# Updated Docs",
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "# Updated Docs", got.Readme)
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
		ArrowMeta:           domain.ArrowMeta{Name: "Test Arrow"},
		InstalledConstraint: "^v2",
		Readme:              "# Docs v2",
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), newNs.String())
	require.NoError(t, err)
	assert.Equal(t, newNs, got.Namespace)
	assert.Equal(t, "v2.0.0", got.Namespace.Ref(), "the upgraded aggregate takes its version from the new ref")
	assert.Equal(t, "^v2", got.InstalledConstraint)
	assert.Equal(t, oldNs, got.UpgradedFromNs)
	assert.False(t, got.UserInstalled)
	assert.Equal(t, "# Docs v2", got.Readme)
}

// ─── Validate helpers ─────────────────────────────────────────────────────────

func isValidationErr(err error) bool {
	return errors.Is(err, asynxModels.ErrValidation) ||
		errors.Is(err, asynxModels.ErrPipelineFailed)
}
