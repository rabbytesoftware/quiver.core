package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

type fixture struct {
	*mocks.Sandbox
	placer *placer
}

func newFixture(
	t *testing.T,
	goos string,
) *fixture {
	t.Helper()
	sb := mocks.NewSandbox(t, goos)
	return &fixture{
		Sandbox: sb,
		placer:  New(sb.Host(), ownership.NewBundles(sb.Tagger)).(*placer),
	}
}

func TestPlacer_Place_DispatchesByPlatform(t *testing.T) {
	mocks.RequireUnixHost(t)
	testCases := []struct {
		name string
		goos string
		want string
	}{
		{name: "windows shim", goos: platform.GOOSWindows, want: "tool.cmd"},
		{name: "unix symlink", goos: "linux", want: "tool"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			wd := f.Workdir(t, mocks.NsA)
			target := filepath.Join(wd, "tool")
			mocks.WriteFile(t, target, "x", 0o755)
			req := models.ApplyRequest{Layout: f.Layout(t), Bare: mocks.BareA, Workdir: wd}

			got, err := f.placer.Place(context.Background(), req, models.Candidate{Name: "tool", Target: target})

			require.NoError(t, err)
			assert.Equal(t, models.Placement{Location: filepath.Join(f.Bin, tc.want)}, got)
			require.NoError(t, f.placer.Remove(req.Layout, mocks.BareA, nil))
			assert.NoFileExists(t, got.Location)
		})
	}
}

func TestPlacer_SymlinkHolder(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	bin := t.TempDir()
	own := filepath.Join(bin, "own")
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "acme", "tool@v1", "x"), own))
	plain := filepath.Join(bin, "plain")
	mocks.WriteFile(t, plain, "x", 0o755)

	testCases := []struct {
		name string
		loc  string
		want ownership.Holder
	}{
		{name: "absent", loc: filepath.Join(bin, "missing"), want: ownership.Holder{}},
		{name: "regular file", loc: plain, want: ownership.Holder{Exists: true}},
		{name: "owned link", loc: own, want: ownership.Holder{Exists: true, Namespace: mocks.BareA}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.placer.symlinkHolder(f.NsDir, tc.loc)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := f.placer.symlinkHolder(f.NsDir, filepath.Join(plain, "child"))
	require.Error(t, err)
}
