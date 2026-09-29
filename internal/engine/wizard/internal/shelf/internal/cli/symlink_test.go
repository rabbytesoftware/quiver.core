package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

func TestPlacer_PlaceSymlink_Refusals(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "dir"), 0o750))
	l := f.Layout(t)
	req := models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}

	testCases := []struct {
		name   string
		target string
		want   string
	}{
		{name: "missing target", target: filepath.Join(wd, "missing"), want: models.ReasonNotFound},
		{name: "directory target", target: filepath.Join(wd, "dir"), want: models.ReasonWrongType},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.placer.placeSymlink(context.Background(), req, models.Candidate{Name: "tool", Target: tc.target})
			require.NoError(t, err)
			assert.Equal(t, models.Placement{Refused: tc.want}, got)
		})
	}
}

func TestPlacer_PlaceSymlink_Errors(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	testCases := []struct {
		name string
		bin  string
	}{
		{name: "location unreadable", bin: file},
		{name: "bin missing", bin: filepath.Join(t.TempDir(), "missing")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := models.ApplyRequest{Layout: platform.Layout{Bin: tc.bin, Namespaces: f.NsDir}, Bare: mocks.BareA, Workdir: wd}
			_, err := f.placer.placeSymlink(context.Background(), req, models.Candidate{Name: "tool", Target: target})
			require.Error(t, err)
		})
	}
}

func TestPlacer_PlaceSymlink_SignsOnDarwinARM64(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, platform.GOOSDarwin)
	f.placer.host.GOARCH = platform.GOARCHARM64
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, string([]byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0}), 0o755)
	l := f.Layout(t)

	got, err := f.placer.placeSymlink(context.Background(), models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}, models.Candidate{Name: "tool", Target: target})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.Bin, "tool"), got.Location)
	assert.Equal(t, [][]string{{"codesign", "-v", target}}, f.Cmd.Calls)
}

func TestPlacer_PlaceSymlink_MarksDeclaredTargets(t *testing.T) {
	mocks.RequireUnixHost(t)
	outsideDir := t.TempDir()

	testCases := []struct {
		name     string
		declared bool
		mode     os.FileMode
		outside  bool
		want     os.FileMode
	}{
		{name: "declared without exec bits gains them", declared: true, mode: 0o644, want: 0o755},
		{name: "declared with an exec bit is left alone", declared: true, mode: 0o740, want: 0o740},
		{name: "auto candidate is never marked", declared: false, mode: 0o644, want: 0o644},
		{name: "declared resolving outside is never marked", declared: true, mode: 0o644, outside: true, want: 0o644},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "linux")
			wd := f.Workdir(t, mocks.NsA)
			l := f.Layout(t)
			real := filepath.Join(wd, "tool")
			target := real
			if tc.outside {
				real = filepath.Join(outsideDir, tc.name)
				require.NoError(t, os.Symlink(real, target))
			}
			mocks.WriteFile(t, real, "x", tc.mode)

			got, err := f.placer.placeSymlink(context.Background(), models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}, models.Candidate{Name: "tool", Target: target, Declared: tc.declared})

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(f.Bin, "tool"), got.Location)
			info, err := os.Stat(real)
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.Mode().Perm())
		})
	}
}

func TestPlacer_PlaceSymlink_MarksResolvedPathOnly(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	l := f.Layout(t)
	real := filepath.Join(wd, "real-tool")
	mocks.WriteFile(t, real, "x", 0o644)
	target := filepath.Join(wd, "tool")
	require.NoError(t, os.Symlink(real, target))
	resolved, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	var chmodded []string
	f.placer.host.Chmod = func(path string, mode os.FileMode) error {
		chmodded = append(chmodded, path)
		return os.Chmod(path, mode)
	}

	_, err = f.placer.placeSymlink(context.Background(), models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}, models.Candidate{Name: "tool", Target: target, Declared: true})

	require.NoError(t, err)
	assert.Equal(t, []string{resolved}, chmodded)
	info, err := os.Lstat(real)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestPlacer_PlaceSymlink_MarkError(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o644)
	l := f.Layout(t)
	f.placer.host.Chmod = func(string, os.FileMode) error { return os.ErrPermission }

	_, err := f.placer.placeSymlink(context.Background(), models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}, models.Candidate{Name: "tool", Target: target, Declared: true})

	require.ErrorIs(t, err, os.ErrPermission)
	_, statErr := os.Lstat(filepath.Join(f.Bin, "tool"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestPlacer_LinkHolder(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	bundle := filepath.Join(f.Apps[0], "Tool.app")
	require.NoError(t, os.MkdirAll(filepath.Join(bundle, "Contents"), 0o750))
	wd := filepath.Join(f.NsDir, "github.com", "acme", "tool@v1")
	require.NoError(t, f.Tagger.Write(bundle, ownership.BundleTag(mocks.BareA, wd, bundle)))
	inWorkdir := filepath.Join(wd, "x")

	testCases := []struct {
		name   string
		goos   string
		target string
		want   ownership.Holder
	}{
		{name: "workdir", goos: "linux", target: inWorkdir, want: ownership.Holder{Exists: true, Namespace: mocks.BareA, Target: inWorkdir}},
		{name: "outside on linux", goos: "linux", target: filepath.Join(bundle, "Contents", "tool"), want: ownership.Holder{Exists: true}},
		{name: "tagged bundle on darwin", goos: platform.GOOSDarwin, target: filepath.Join(bundle, "Contents", "tool"), want: ownership.Holder{Exists: true, Namespace: mocks.BareA, Target: wd}},
		{name: "untagged on darwin", goos: platform.GOOSDarwin, target: filepath.Join(f.Apps[0], "Other.app", "tool"), want: ownership.Holder{Exists: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f.placer.host.GOOS = tc.goos
			assert.Equal(t, tc.want, f.placer.linkHolder(f.NsDir, tc.target))
		})
	}
}

func TestPlacer_RemoveSymlinks(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	own := filepath.Join(f.Bin, "own")
	foreign := filepath.Join(f.Bin, "foreign")
	plain := filepath.Join(f.Bin, "plain")
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "acme", "tool@v1", "own"), own))
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "other", "thing@v1", "x"), foreign))
	mocks.WriteFile(t, plain, "x", 0o755)
	kept := filepath.Join(f.Bin, "kept")
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "acme", "tool@v1", "kept"), kept))
	l := f.Layout(t)

	require.NoError(t, f.placer.removeSymlinks(l, ownership.NamespaceClaim(mocks.BareA), map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	_, err := os.Lstat(kept)
	assert.NoError(t, err)
	_, err = os.Lstat(foreign)
	assert.NoError(t, err)
	assert.FileExists(t, plain)
}

func TestPlacer_RemoveSymlinks_Errors(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.NoError(t, f.placer.removeSymlinks(platform.Layout{Bin: filepath.Join(t.TempDir(), "missing")}, ownership.NamespaceClaim(mocks.BareA), nil))
	assert.Error(t, f.placer.removeSymlinks(platform.Layout{Bin: file}, ownership.NamespaceClaim(mocks.BareA), nil))

	if os.Geteuid() == 0 {
		return
	}
	bin := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "acme", "tool@v1", "x"), filepath.Join(bin, "own")))
	require.NoError(t, os.Chmod(bin, 0o500))
	t.Cleanup(func() { _ = os.Chmod(bin, 0o750) })

	assert.Error(t, f.placer.removeSymlinks(platform.Layout{Bin: bin, Namespaces: f.NsDir}, ownership.NamespaceClaim(mocks.BareA), nil))
}

func TestPlacer_RemoveSymlinks_WorkdirClaimSparesSiblingRef(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	v1 := f.Workdir(t, mocks.NsA)
	v2 := f.Workdir(t, mocks.NsA2)
	mocks.WriteFile(t, filepath.Join(v1, "old"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(v2, "tool"), "x", 0o755)
	mine := filepath.Join(f.Bin, "old")
	sibling := filepath.Join(f.Bin, "tool")
	stale := filepath.Join(f.Bin, "stale")
	require.NoError(t, os.Symlink(filepath.Join(v1, "old"), mine))
	require.NoError(t, os.Symlink(filepath.Join(v2, "tool"), sibling))
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "acme", "tool@v0", "tool"), stale))

	require.NoError(t, f.placer.removeSymlinks(f.Layout(t), ownership.WorkdirClaim(mocks.BareA, v1), nil))

	_, err := os.Lstat(mine)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Lstat(stale)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Lstat(sibling)
	assert.NoError(t, err)
}
