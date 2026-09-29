package desktop

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
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

func appRequest(
	t *testing.T,
	f *fixture,
	ns domain.Namespace,
) (models.ApplyRequest, string) {
	t.Helper()
	wd := f.Workdir(t, ns)
	return models.ApplyRequest{Layout: f.Layout(t), Bare: ns.BareNamespace(), Workdir: wd, Moved: map[string]string{}}, wd
}

func TestPlacer_Place_DispatchesByPlatform(t *testing.T) {
	mocks.RequireUnixHost(t)
	testCases := []struct {
		name  string
		goos  string
		setup func(t *testing.T, f *fixture, wd string) (models.Candidate, string)
	}{
		{
			name: "darwin bundle",
			goos: platform.GOOSDarwin,
			setup: func(t *testing.T, f *fixture, wd string) (models.Candidate, string) {
				mocks.WriteBundle(t, filepath.Join(wd, "Tool.app"), "v1")
				return models.Candidate{Name: "Tool", Target: filepath.Join(wd, "Tool.app")}, filepath.Join(f.Apps[0], "Tool.app")
			},
		},
		{
			name: "windows shortcut",
			goos: platform.GOOSWindows,
			setup: func(t *testing.T, f *fixture, wd string) (models.Candidate, string) {
				mocks.WriteFile(t, filepath.Join(wd, "Tool.exe"), "x", 0o755)
				return models.Candidate{Name: "Tool", Target: filepath.Join(wd, "Tool.exe")}, filepath.Join(lnkDir(filepath.Join(f.UserHome, "AppData", "Roaming")), "Tool.lnk")
			},
		},
		{
			name: "linux entry",
			goos: "linux",
			setup: func(t *testing.T, f *fixture, wd string) (models.Candidate, string) {
				mocks.WriteFile(t, filepath.Join(wd, "Tool.AppImage"), "x", 0o755)
				return models.Candidate{Name: "Tool", Target: filepath.Join(wd, "Tool.AppImage")}, filepath.Join(xdgDir(f.UserHome), xdgFileName(mocks.BareA, "Tool"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			req, wd := appRequest(t, f, mocks.NsA)
			c, want := tc.setup(t, f, wd)
			f.Cmd.Respond = func(_ string, _, env []string) ([]byte, error) {
				if tc.goos == platform.GOOSWindows {
					mocks.WriteFile(t, want, "", 0o600)
				}
				return nil, nil
			}

			got, err := f.placer.Place(context.Background(), req, domain.ExposeEntry{}, c)

			require.NoError(t, err)
			assert.Equal(t, models.Placement{Location: want}, got)
			require.NoError(t, f.placer.Remove(context.Background(), req.Layout, ownership.NamespaceClaim(mocks.BareA), nil))
		})
	}
}
