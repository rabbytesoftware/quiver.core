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

// ─── MarkLastUsed ────────────────────────────────────────────────────────────

func TestMarkLastUsed_WithoutPriorAdd_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	cmd := commands.MarkLastUsed{
		Namespace:  ns,
		LastUsedAt: time.Now(),
	}
	_, err := ax.Send(context.Background(), cmd)
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestMarkLastUsed_AfterAdd_StampsLastUsedAt(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	now := time.Now().UTC().Truncate(time.Second)
	cmd := commands.MarkLastUsed{
		Namespace:  ns,
		LastUsedAt: now,
	}
	_, err := ax.Send(context.Background(), cmd)
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, now, got.LastUsedAt.UTC().Truncate(time.Second))
	assert.Equal(t, "v1.0.0", got.Namespace.Ref(), "the stamped ref is the one the aggregate is keyed by")
}

// Which ref an execute ran against is answered by which aggregate carries the
// stamp: the command routes on the full namespace@ref, so a sibling ref of the
// same repo stays untouched.
func TestMarkLastUsed_StampsOnlyTheRefItNames(t *testing.T) {
	ax := buildAsynx(t)
	installed := domain.Namespace("github.com/user/repo@v1.2.3")
	sibling := domain.Namespace("github.com/user/repo@v2.0.0")
	seedArrow(t, ax, installed, false)
	seedArrow(t, ax, sibling, false)

	_, err := ax.Send(context.Background(), commands.MarkLastUsed{
		Namespace:  installed,
		LastUsedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), installed.String())
	require.NoError(t, err)
	assert.False(t, got.LastUsedAt.IsZero())

	other, err := ax.Get(context.Background(), sibling.String())
	require.NoError(t, err)
	assert.True(t, other.LastUsedAt.IsZero(), "running one ref must not stamp another")
}

// A re-run at the same ref must overwrite the stamp rather than accumulate
// state, so replaying the command twice is indistinguishable from once.
func TestMarkLastUsed_Reapplied_OverwritesTheStamp(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedArrow(t, ax, ns, false)

	first := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	_, err := ax.Send(context.Background(), commands.MarkLastUsed{
		Namespace:  ns,
		LastUsedAt: first,
	})
	require.NoError(t, err)

	second := time.Now().UTC().Truncate(time.Second)
	_, err = ax.Send(context.Background(), commands.MarkLastUsed{
		Namespace:  ns,
		LastUsedAt: second,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, second, got.LastUsedAt.UTC().Truncate(time.Second))
}

func TestMarkLastUsed_CommandContract(t *testing.T) {
	cmd := commands.MarkLastUsed{Namespace: testNs()}

	assert.Equal(t, testNs().String(), cmd.AggregateID())
	assert.Equal(t, "arrow.last_used."+testNs().String(), cmd.EventName())
	assert.True(t, cmd.ShouldSnapshot(), "a last-used stamp is a durable transition")
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
// the intent to keep the arrow around, and the selector state is written when
// the namespace is added, so an update can still resolve through it.
func TestMarkUninstalled_KeepsTheAddTimeFields(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	_, err := ax.Send(context.Background(), commands.AddArrow{
		Namespace:     ns,
		ArrowMeta:     domain.ArrowMeta{Name: "Test Arrow"},
		Variables:     []domain.Variable{{Name: "PORT"}},
		DirectInstall: true,
		SelectorKind:  domain.SelectorConstraint,
		Resolved:      domain.Resolved{Ref: "v1.2.0", Commit: "c1"},
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
	assert.Equal(t, domain.SelectorConstraint, got.SelectorKind)
	assert.Equal(t, domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, got.Resolved)
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

// ─── AddArrow selector state ─────────────────────────────────────────────────

func TestAddArrow_SelectorKindAndResolved_RoundTrip(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/repo@stable")
	resolved := domain.Resolved{Ref: "v1.2.0", Commit: "abc123", Fingerprint: "sha256:ff"}

	_, err := ax.Send(context.Background(), commands.AddArrow{
		Namespace:     ns,
		DirectInstall: true,
		SelectorKind:  domain.SelectorChannel,
		Resolved:      resolved,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.SelectorChannel, got.SelectorKind)
	assert.Equal(t, resolved, got.Resolved)
	assert.Nil(t, got.Available)
}

// ─── AdvanceArrow ────────────────────────────────────────────────────────────

func TestAdvanceArrow_WithoutPriorAdd_Fails(t *testing.T) {
	ax := buildAsynx(t)

	_, err := ax.Send(context.Background(), commands.AdvanceArrow{Namespace: testNs()})

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestAdvanceArrow_ReplacesManifestAndResolved_PreservesRowState(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/repo@nightly-latest")
	installedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	lastUsedAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	_, err := ax.Send(context.Background(), commands.AddArrow{
		Namespace:     ns,
		ArrowMeta:     domain.ArrowMeta{Name: "Old"},
		Readme:        "old readme",
		DirectInstall: true,
		SelectorKind:  domain.SelectorPin,
		Resolved:      domain.Resolved{Ref: "nightly-latest", Commit: "old111", Fingerprint: "old111"},
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.MarkInstalled{Namespace: ns, InstalledAt: installedAt})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.MarkLastUsed{Namespace: ns, LastUsedAt: lastUsedAt})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.RecordAvailable{
		Namespace: ns,
		Available: &domain.Available{Ref: "nightly-latest", Commit: "new222"},
	})
	require.NoError(t, err)

	target := domain.Resolved{Ref: "nightly-latest", Commit: "new222", Fingerprint: "new222"}
	targets := map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}}
	variables := []domain.Variable{{Name: "PORT"}}
	_, err = ax.Send(context.Background(), commands.AdvanceArrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "New"},
		Variables: variables,
		Targets:   targets,
		Readme:    "new readme",
		Resolved:  target,
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ns, got.Namespace)
	assert.Equal(t, "New", got.Name)
	assert.Equal(t, "new readme", got.Readme)
	assert.Equal(t, variables, got.Variables)
	assert.Equal(t, targets, got.Targets)
	assert.Equal(t, target, got.Resolved)
	assert.Nil(t, got.Available)
	assert.True(t, installedAt.Equal(got.InstalledAt))
	assert.True(t, lastUsedAt.Equal(got.LastUsedAt))
	assert.True(t, got.UserInstalled)
	assert.Equal(t, domain.SelectorPin, got.SelectorKind)
}

func TestAdvanceArrow_PreservesStoredSelectorKind(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/repo@v1.*")
	_, err := ax.Send(context.Background(), commands.AddArrow{
		Namespace:    ns,
		SelectorKind: domain.SelectorConstraint,
	})
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), commands.AdvanceArrow{
		Namespace: ns,
		Resolved:  domain.Resolved{Ref: "v1.3.0", Commit: "c3"},
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.SelectorConstraint, got.SelectorKind)
	assert.Equal(t, "v1.3.0", got.Resolved.Ref)
}

// ─── RecordAvailable ─────────────────────────────────────────────────────────

func TestRecordAvailable_WithoutPriorAdd_Fails(t *testing.T) {
	ax := buildAsynx(t)

	_, err := ax.Send(context.Background(), commands.RecordAvailable{
		Namespace: testNs(),
		Available: &domain.Available{Ref: "v1.1.0", Commit: "c1"},
	})

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestRecordAvailable_SetsAndClears(t *testing.T) {
	testCases := []struct {
		name      string
		available []*domain.Available
		want      *domain.Available
	}{
		{
			name:      "sets the available ref",
			available: []*domain.Available{{Ref: "v1.1.0", Commit: "c1"}},
			want:      &domain.Available{Ref: "v1.1.0", Commit: "c1"},
		},
		{
			name:      "a later check overwrites the earlier one",
			available: []*domain.Available{{Ref: "v1.1.0", Commit: "c1"}, {Ref: "v1.2.0", Commit: "c2"}},
			want:      &domain.Available{Ref: "v1.2.0", Commit: "c2"},
		},
		{
			name:      "nil clears it",
			available: []*domain.Available{{Ref: "v1.1.0", Commit: "c1"}, nil},
			want:      nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := buildAsynx(t)
			ns := testNs()
			seedArrow(t, ax, ns, true)

			for _, a := range tc.available {
				_, err := ax.Send(context.Background(), commands.RecordAvailable{Namespace: ns, Available: a})
				require.NoError(t, err)
			}

			got, err := ax.Get(context.Background(), ns.String())
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Available)
			assert.Equal(t, "Test Arrow", got.Name)
			assert.True(t, got.UserInstalled)
		})
	}
}

// ─── Validate helpers ─────────────────────────────────────────────────────────

func isValidationErr(err error) bool {
	return errors.Is(err, asynxModels.ErrValidation) ||
		errors.Is(err, asynxModels.ErrPipelineFailed)
}

func TestRefreshManifest_WithoutPriorAdd_Fails(t *testing.T) {
	ax := buildAsynx(t)

	_, err := ax.Send(context.Background(), commands.RefreshManifest{Namespace: testNs()})

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// A refresh stages the target's manifest for its update: what is installed
// (Resolved) and what the check found ahead (Available) stay until the
// update commits.
func TestRefreshManifest_ReplacesOnlyTheManifest(t *testing.T) {
	ax := buildAsynx(t)
	ns := domain.Namespace("github.com/user/repo@stable")
	resolved := domain.Resolved{Ref: "v1.0.0", Commit: "c1", Fingerprint: "c1"}
	available := &domain.Available{Ref: "v1.1.0", Commit: "c2"}

	_, err := ax.Send(context.Background(), commands.AddArrow{
		Namespace:     ns,
		ArrowMeta:     domain.ArrowMeta{Name: "Old"},
		Readme:        "old readme",
		DirectInstall: true,
		SelectorKind:  domain.SelectorChannel,
		Resolved:      resolved,
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.RecordAvailable{Namespace: ns, Available: available})
	require.NoError(t, err)

	targets := map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}}
	variables := []domain.Variable{{Name: "PORT"}}
	evt, err := ax.Send(context.Background(), commands.RefreshManifest{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "New"},
		Variables: variables,
		Targets:   targets,
		Readme:    "new readme",
	})
	require.NoError(t, err)
	assert.Equal(t, "arrow.manifest_refreshed."+ns.String(), evt.EventName)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ns, got.Namespace)
	assert.Equal(t, "New", got.Name)
	assert.Equal(t, "new readme", got.Readme)
	assert.Equal(t, variables, got.Variables)
	assert.Equal(t, targets, got.Targets)
	assert.Equal(t, resolved, got.Resolved)
	assert.Equal(t, available, got.Available)
	assert.Equal(t, domain.SelectorChannel, got.SelectorKind)
	assert.True(t, got.UserInstalled)
}
