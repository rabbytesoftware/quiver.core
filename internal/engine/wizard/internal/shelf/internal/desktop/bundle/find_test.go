package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func TestExposer_Find(t *testing.T) {
	testCases := []struct {
		name       string
		dirs       []string
		files      []string
		placed     map[string]domain.Namespace
		entryName  string
		want       *models.Candidate
		wantReason string
	}{
		{
			name:      "single bundle keeps its own name",
			dirs:      []string{"CC Switch.app"},
			entryName: "cc-switch",
			want:      &models.Candidate{Name: "CC Switch", Target: "CC Switch.app", Depth: 1},
		},
		{
			name:      "bundle already moved resolves to the bundle this arrow placed",
			placed:    map[string]domain.Namespace{"CC Switch.app": mocks.BareA, "Other.app": mocks.BareB},
			entryName: "cc-switch",
			want:      &models.Candidate{Name: "CC Switch", Target: "CC Switch.app", Depth: 1},
		},
		{
			name:       "bundle placed by another arrow is not ours",
			placed:     map[string]domain.Namespace{"Other.app": mocks.BareB},
			entryName:  "Tool",
			wantReason: models.ReasonNoDesktop,
		},
		{
			name:       "nothing and no name",
			wantReason: models.ReasonNoDesktop,
		},
		{
			name: "several bundles pick the repo name",
			dirs: []string{"Other.app", "Tool.app"},
			want: &models.Candidate{Name: "Tool", Target: "Tool.app", Depth: 1},
		},
		{
			name:      "bundle inside a wrapper directory",
			dirs:      []string{"Tool-1.0/Tool.app"},
			entryName: "tool",
			want:      &models.Candidate{Name: "Tool", Target: "Tool-1.0/Tool.app", Depth: 2},
		},
		{
			name:       "several bundles without a match",
			dirs:       []string{"One.app", "Two.app"},
			wantReason: models.ReasonAmbiguous,
		},
		{
			name:       "no scan fallback for executables",
			files:      []string{"logseq/Logseq"},
			entryName:  "logseq",
			wantReason: models.ReasonNoDesktop,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			req, wd := f.Request(t, mocks.NsA)
			for _, d := range tc.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(wd, d), 0o750))
			}
			for _, file := range tc.files {
				mocks.WriteFile(t, filepath.Join(wd, filepath.FromSlash(file)), "x", 0o755)
			}
			for bundle, owner := range tc.placed {
				path := filepath.Join(f.Apps[0], bundle)
				require.NoError(t, os.MkdirAll(path, 0o750))
				require.NoError(t, f.Tagger.Write(path, ownership.BundleTag(owner, "/wd", path)))
			}
			mocks.WriteFile(t, filepath.Join(f.Apps[0], "Stray.app"), "x", 0o644)

			got, reason, err := f.exposer.Find(req, domain.ExposeEntry{Name: tc.entryName, Path: domain.ExposeAuto})

			require.NoError(t, err)
			assert.Equal(t, tc.wantReason, reason)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			want := *tc.want
			want.Target = filepath.Join(wd, want.Target)
			assert.Equal(t, []models.Candidate{want}, got)
		})
	}
}

func TestExposer_Find_PrefersTheRecord(t *testing.T) {
	f := newFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "Other.app"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "dist", "Logseq.app"), 0o750))
	mocks.WriteRecord(t, wd, domain.PortableApp{Name: "Display Name", Entry: "dist/Logseq.app"})

	got, reason, err := f.exposer.Find(req, domain.ExposeEntry{Name: "tool", Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []models.Candidate{{Name: "Logseq", Display: "Display Name", Target: filepath.Join(wd, "dist", "Logseq.app"), Depth: 1}}, got)
}

func TestExposer_Find_IgnoresSymlinkedBundles(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	outside := filepath.Join(t.TempDir(), "X.app")
	require.NoError(t, os.MkdirAll(outside, 0o750))
	require.NoError(t, os.Symlink(outside, filepath.Join(wd, "X.app")))

	got, reason, err := f.exposer.Find(req, domain.ExposeEntry{Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, models.ReasonNoDesktop, reason)
}

func TestExposer_Find_MissingWorkdir(t *testing.T) {
	f := newFixture(t)
	req := models.Request{Bare: mocks.BareA, Workdir: filepath.Join(t.TempDir(), "missing")}

	_, _, err := f.exposer.Find(req, domain.ExposeEntry{Name: "x", Path: domain.ExposeAuto})

	require.Error(t, err)
}
