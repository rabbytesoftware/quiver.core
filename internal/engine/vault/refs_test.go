package vault

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestRefs_NothingSaved_IsNotCached(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetRefs(t.Context(), "github.com/u/r")

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestRefs_PutThenGet_ReturnsTheSnapshotStampedNow(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	s.clock = func() time.Time { return now }
	snap := domain.RefSnapshot{Tags: map[string]string{"v1.0.0": "c1"}, Branches: map[string]string{"main": "c2"}, Head: "main"}

	require.NoError(t, s.PutRefs(t.Context(), "github.com/u/r@stable", snap))
	got, err := s.GetRefs(t.Context(), "github.com/u/r")

	require.NoError(t, err)
	assert.Equal(t, snap, got.Snapshot)
	assert.True(t, now.Equal(got.CachedAt))
}

func TestRefs_AreKeptPerRepositoryAndOutliveTheirAge(t *testing.T) {
	s := newTestStore(t)
	s.clock = func() time.Time { return time.Now().Add(-1000 * time.Hour) }
	require.NoError(t, s.PutRefs(t.Context(), "github.com/u/a", domain.RefSnapshot{Head: "a"}))
	require.NoError(t, s.PutRefs(t.Context(), "github.com/u/b", domain.RefSnapshot{Head: "b"}))

	a, errA := s.GetRefs(t.Context(), "github.com/u/a")
	b, errB := s.GetRefs(t.Context(), "github.com/u/b")

	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.Equal(t, "a", a.Snapshot.Head)
	assert.Equal(t, "b", b.Snapshot.Head)
}

func TestRefs_APutReplacesTheLast(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.PutRefs(t.Context(), "github.com/u/r", domain.RefSnapshot{Head: "old"}))
	require.NoError(t, s.PutRefs(t.Context(), "github.com/u/r", domain.RefSnapshot{Head: "new"}))

	got, err := s.GetRefs(t.Context(), "github.com/u/r")

	require.NoError(t, err)
	assert.Equal(t, "new", got.Snapshot.Head)
}

func TestRefs_TornFile_IsNotCached(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, os.WriteFile(s.refsFilePath("github.com/u/r"), []byte("{torn"), 0o600))

	_, err := s.GetRefs(t.Context(), "github.com/u/r")

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestRefs_UnreadableFile_ReportsTheError(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, os.Mkdir(s.refsFilePath("github.com/u/r"), 0o700))

	_, err := s.GetRefs(t.Context(), "github.com/u/r")

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
}

func TestRefs_InvalidNamespace_IsRefused(t *testing.T) {
	s := newTestStore(t)

	_, getErr := s.GetRefs(t.Context(), "not a namespace")
	putErr := s.PutRefs(t.Context(), "not a namespace", domain.RefSnapshot{})

	assert.ErrorIs(t, getErr, ErrInvalidNamespace)
	assert.ErrorIs(t, putErr, ErrInvalidNamespace)
}

func TestRefs_UnwritableDir_ReportsTheError(t *testing.T) {
	s := newTestStore(t)
	blocker := s.vaultPath + "/file"
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	s.vaultPath = blocker + "/sub"

	err := s.PutRefs(t.Context(), "github.com/u/r", domain.RefSnapshot{})

	require.Error(t, err)
}
