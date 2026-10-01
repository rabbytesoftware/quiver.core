package versioning

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type fixedRefs struct {
	snap domain.RefSnapshot
}

func (f fixedRefs) Refs(_ context.Context, _ domain.Namespace) (domain.RefSnapshot, error) {
	return f.snap, nil
}

func TestVersioning_PublicAPIDelegates(t *testing.T) {
	snap := domain.RefSnapshot{
		Tags:     map[string]string{"v1.0.0": "c100", "v1.1.0": "c110"},
		Branches: map[string]string{"main": "cm"},
		Head:     "main",
	}

	got, err := New(fixedRefs{snap: snap}, time.Now, time.Hour).Snapshot(context.Background(), "github.com/u/r")
	require.NoError(t, err)
	assert.Equal(t, snap, got)

	latest, ok := LatestStable(snap)
	assert.True(t, ok)
	assert.Equal(t, "v1.1.0", latest)

	branch, commit, ok := DefaultBranch(snap)
	assert.True(t, ok)
	assert.Equal(t, "main", branch)
	assert.Equal(t, "cm", commit)

	commit, ok = RefCommit(domain.SelectorOrderedChannel, "stable", "v1.0.0", snap)
	assert.True(t, ok)
	assert.Equal(t, "c100", commit)

	assert.True(t, HasEmptyComponent("a//b"))
	assert.False(t, HasEmptyComponent("a/b"))
}
