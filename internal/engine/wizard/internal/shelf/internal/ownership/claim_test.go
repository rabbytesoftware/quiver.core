package ownership

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func TestWorkdirOwner(t *testing.T) {
	nsDir := filepath.Join(t.TempDir(), "namespaces")

	testCases := []struct {
		name   string
		target string
		want   domain.Namespace
	}{
		{
			name:   "versioned workdir",
			target: filepath.Join(nsDir, "github.com", "u", "r@v1", "bin", "rg"),
			want:   "github.com/u/r",
		},
		{
			name:   "workdir itself",
			target: filepath.Join(nsDir, "github.com", "u", "r@v1"),
			want:   "github.com/u/r",
		},
		{
			name:   "quiver hosted",
			target: filepath.Join(nsDir, "q.io", "u", "r", "auid@v2", "x"),
			want:   "q.io/u/r/auid",
		},
		{
			name:   "ref with slash",
			target: filepath.Join(nsDir, "github.com", "u", "r@feature", "x", "rg"),
			want:   "github.com/u/r",
		},
		{
			name:   "no ref",
			target: filepath.Join(nsDir, "github.com", "u", "r", "rg"),
			want:   "",
		},
		{
			name:   "outside",
			target: filepath.Join(filepath.Dir(nsDir), "elsewhere@v1", "rg"),
			want:   "",
		},
		{
			name:   "namespaces dir",
			target: nsDir,
			want:   "",
		},
		{
			name:   "relative",
			target: "relative@v1/rg",
			want:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, WorkdirOwner(nsDir, tc.target))
		})
	}
}

func TestHolder_ClaimedBy_Namespace(t *testing.T) {
	claim := models.NamespaceClaim(mocks.BareA)

	assert.True(t, Holder{Exists: true, Namespace: mocks.BareA, Target: "/anywhere"}.ClaimedBy(claim))
	assert.False(t, Holder{Exists: true, Namespace: mocks.BareB}.ClaimedBy(claim))
	assert.False(t, Holder{Exists: true}.ClaimedBy(claim))
}

func TestHolder_ClaimedBy_Workdir(t *testing.T) {
	nsDir := t.TempDir()
	v1 := filepath.Join(nsDir, "github.com", "acme", "tool@v1")
	v2 := filepath.Join(nsDir, "github.com", "acme", "tool@v2")
	mocks.WriteFile(t, filepath.Join(v1, "tool"), "v1", 0o600)
	mocks.WriteFile(t, filepath.Join(v2, "tool"), "v2", 0o600)
	claim := models.WorkdirClaim(mocks.BareA, v1)

	testCases := []struct {
		name string
		h    Holder
		want bool
	}{
		{name: "entry pointing into the workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: filepath.Join(v1, "tool")}, want: true},
		{name: "marker naming the workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: v1}, want: true},
		{name: "entry pointing into a sibling ref's live workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: filepath.Join(v2, "tool")}},
		{name: "marker naming a sibling ref's live workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: v2}},
		{name: "entry pointing into a vanished workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: filepath.Join(nsDir, "github.com", "acme", "tool@v0", "tool")}, want: true},
		{name: "entry recording no workdir", h: Holder{Exists: true, Namespace: mocks.BareA}, want: true},
		{name: "another namespace pointing into the workdir", h: Holder{Exists: true, Namespace: mocks.BareB, Target: filepath.Join(v1, "tool")}},
		{name: "unmanaged entry", h: Holder{Exists: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.h.ClaimedBy(claim))
		})
	}
}
