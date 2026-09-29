package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

type symlinkFixture struct {
	*mocks.Sandbox
	symlink *symlink
}

func newSymlinkFixture(
	t *testing.T,
) *symlinkFixture {
	t.Helper()
	sb := mocks.NewSandbox(t, "linux")
	return &symlinkFixture{
		Sandbox: sb,
		symlink: NewSymlink(sb.Host(), discover.Scan{Rule: discover.ExecBits()}, Unsigned(), ownership.Unowned).(*symlink),
	}
}

func (f *symlinkFixture) place(
	t *testing.T,
	req models.Request,
	c models.Candidate,
) (models.Placement, error) {
	t.Helper()
	return f.symlink.Place(context.Background(), req, domain.ExposeEntry{}, c)
}

func TestSymlink_Find(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "tool"), "x", 0o755)

	got, reason, err := f.symlink.Find(req, domain.ExposeEntry{Name: "tool", Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []models.Candidate{{Name: "tool", Target: filepath.Join(wd, "tool"), Depth: 1}}, got)
}

func TestSymlink_PlaceAndRemove(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o755)

	got, err := f.place(t, req, models.Candidate{Name: "tool", Target: target})

	require.NoError(t, err)
	assert.Equal(t, models.Placement{Location: filepath.Join(f.Bin, "tool")}, got)
	link, err := os.Readlink(got.Location)
	require.NoError(t, err)
	assert.Equal(t, target, link)
	require.NoError(t, f.symlink.Remove(context.Background(), req.Layout, models.NamespaceClaim(mocks.BareA), nil))
	assert.NoFileExists(t, got.Location)
}

func TestSymlink_Place_Refusals(t *testing.T) {
	f := newSymlinkFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "dir"), 0o750))

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
			got, err := f.place(t, req, models.Candidate{Name: "tool", Target: tc.target})
			require.NoError(t, err)
			assert.Equal(t, models.Placement{Refused: tc.want}, got)
		})
	}
}

func TestSymlink_Place_Errors(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
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
			req := models.Request{Layout: models.Layout{Bin: tc.bin, Namespaces: f.NsDir}, Bare: mocks.BareA, Workdir: wd}
			_, err := f.place(t, req, models.Candidate{Name: "tool", Target: target})
			require.Error(t, err)
		})
	}
}

func TestSymlink_Place_Signs(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
	f.symlink.signer = Codesign(f.Cmd)
	req, wd := f.Request(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, string([]byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0}), 0o755)

	got, err := f.place(t, req, models.Candidate{Name: "tool", Target: target})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.Bin, "tool"), got.Location)
	assert.Equal(t, [][]string{{"codesign", "-v", target}}, f.Cmd.Calls)
}

func TestSymlink_Place_MarksDeclaredTargets(t *testing.T) {
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
			f := newSymlinkFixture(t)
			req, wd := f.Request(t, mocks.NsA)
			real := filepath.Join(wd, "tool")
			target := real
			if tc.outside {
				real = filepath.Join(outsideDir, tc.name)
				require.NoError(t, os.Symlink(real, target))
			}
			mocks.WriteFile(t, real, "x", tc.mode)

			got, err := f.place(t, req, models.Candidate{Name: "tool", Target: target, Declared: tc.declared})

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(f.Bin, "tool"), got.Location)
			info, err := os.Stat(real)
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.Mode().Perm())
		})
	}
}

func TestSymlink_Place_MarksResolvedPathOnly(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	real := filepath.Join(wd, "real-tool")
	mocks.WriteFile(t, real, "x", 0o644)
	target := filepath.Join(wd, "tool")
	require.NoError(t, os.Symlink(real, target))
	resolved, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	var chmodded []string
	f.symlink.host.Chmod = func(path string, mode os.FileMode) error {
		chmodded = append(chmodded, path)
		return os.Chmod(path, mode)
	}

	_, err = f.place(t, req, models.Candidate{Name: "tool", Target: target, Declared: true})

	require.NoError(t, err)
	assert.Equal(t, []string{resolved}, chmodded)
	info, err := os.Lstat(real)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestSymlink_Place_MarkError(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	target := filepath.Join(wd, "tool")
	mocks.WriteFile(t, target, "x", 0o644)
	f.symlink.host.Chmod = func(string, os.FileMode) error { return os.ErrPermission }

	_, err := f.place(t, req, models.Candidate{Name: "tool", Target: target, Declared: true})

	require.ErrorIs(t, err, os.ErrPermission)
	_, statErr := os.Lstat(filepath.Join(f.Bin, "tool"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestSymlink_Holder(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
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
		{name: "owned link", loc: own, want: ownership.Holder{Exists: true, Namespace: mocks.BareA, Target: filepath.Join(f.NsDir, "github.com", "acme", "tool@v1", "x")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.symlink.holder(f.NsDir, tc.loc)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := f.symlink.holder(f.NsDir, filepath.Join(plain, "child"))
	require.Error(t, err)
}

func TestSymlink_LinkHolder(t *testing.T) {
	f := newSymlinkFixture(t)
	bundle := filepath.Join(f.Apps[0], "Tool.app")
	require.NoError(t, os.MkdirAll(filepath.Join(bundle, "Contents"), 0o750))
	wd := filepath.Join(f.NsDir, "github.com", "acme", "tool@v1")
	require.NoError(t, f.Tagger.Write(bundle, ownership.BundleTag(mocks.BareA, wd, bundle)))
	inWorkdir := filepath.Join(wd, "x")
	bundles := ownership.NewBundles(f.Tagger)

	testCases := []struct {
		name      string
		enclosing func(string) ownership.Holder
		target    string
		want      ownership.Holder
	}{
		{name: "workdir", enclosing: ownership.Unowned, target: inWorkdir, want: ownership.Holder{Exists: true, Namespace: mocks.BareA, Target: inWorkdir}},
		{name: "outside, nothing encloses it", enclosing: ownership.Unowned, target: filepath.Join(bundle, "Contents", "tool"), want: ownership.Holder{Exists: true}},
		{name: "inside a tagged bundle", enclosing: bundles.Enclosing, target: filepath.Join(bundle, "Contents", "tool"), want: ownership.Holder{Exists: true, Namespace: mocks.BareA, Target: wd}},
		{name: "inside an untagged bundle", enclosing: bundles.Enclosing, target: filepath.Join(f.Apps[0], "Other.app", "tool"), want: ownership.Holder{Exists: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f.symlink.enclosing = tc.enclosing
			assert.Equal(t, tc.want, f.symlink.linkHolder(f.NsDir, tc.target))
		})
	}
}

func TestSymlink_Remove(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
	own := filepath.Join(f.Bin, "own")
	foreign := filepath.Join(f.Bin, "foreign")
	plain := filepath.Join(f.Bin, "plain")
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "acme", "tool@v1", "own"), own))
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "other", "thing@v1", "x"), foreign))
	mocks.WriteFile(t, plain, "x", 0o755)
	kept := filepath.Join(f.Bin, "kept")
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "acme", "tool@v1", "kept"), kept))

	require.NoError(t, f.symlink.Remove(context.Background(), f.Layout(t), models.NamespaceClaim(mocks.BareA), map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	_, err := os.Lstat(kept)
	assert.NoError(t, err)
	_, err = os.Lstat(foreign)
	assert.NoError(t, err)
	assert.FileExists(t, plain)
}

func TestSymlink_Remove_Errors(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	claim := models.NamespaceClaim(mocks.BareA)

	assert.NoError(t, f.symlink.Remove(context.Background(), models.Layout{Bin: filepath.Join(t.TempDir(), "missing")}, claim, nil))
	assert.Error(t, f.symlink.Remove(context.Background(), models.Layout{Bin: file}, claim, nil))

	if os.Geteuid() == 0 {
		return
	}
	bin := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(f.NsDir, "github.com", "acme", "tool@v1", "x"), filepath.Join(bin, "own")))
	require.NoError(t, os.Chmod(bin, 0o500))
	t.Cleanup(func() { _ = os.Chmod(bin, 0o750) })

	assert.Error(t, f.symlink.Remove(context.Background(), models.Layout{Bin: bin, Namespaces: f.NsDir}, claim, nil))
}

func TestSymlink_Remove_WorkdirClaimSparesSiblingRef(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newSymlinkFixture(t)
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

	require.NoError(t, f.symlink.Remove(context.Background(), f.Layout(t), models.WorkdirClaim(mocks.BareA, v1), nil))

	_, err := os.Lstat(mine)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Lstat(stale)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Lstat(sibling)
	assert.NoError(t, err)
}
