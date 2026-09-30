package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/userpath"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

type pathDirFixture struct {
	*mocks.Sandbox
	exposer models.Exposer
}

func newPathDirFixture(
	t *testing.T,
) *pathDirFixture {
	t.Helper()
	sb := mocks.NewSandbox(t, "windows")
	return &pathDirFixture{
		Sandbox: sb,
		exposer: NewPathDir(discover.Scan{Rule: discover.ExeSuffix()}, userpath.NewEditor(sb.UserPath)),
	}
}

func (f *pathDirFixture) place(
	req models.Request,
	c models.Candidate,
) (models.Placement, error) {
	return f.exposer.Place(context.Background(), req, domain.ExposeEntry{}, c)
}

func (f *pathDirFixture) entries() []string {
	return strings.Split(f.UserPath.Value, userpath.Separator)
}

func TestPathDir_Find(t *testing.T) {
	f := newPathDirFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "bin", "tool.exe"), "x", 0o644)

	got, reason, err := f.exposer.Find(req, domain.ExposeEntry{Name: "tool", Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []models.Candidate{{Name: "tool", Target: filepath.Join(wd, "bin", "tool.exe"), Depth: 2}}, got)
}

func TestPathDir_Place_AppendsTheExecutablesFolderOnce(t *testing.T) {
	f := newPathDirFixture(t)
	f.UserPath.Value = `C:\Windows;%USERPROFILE%\tools`
	req, wd := f.Request(t, mocks.NsA)
	bin := filepath.Join(wd, "bin")
	mocks.WriteFile(t, filepath.Join(bin, "tool.exe"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(bin, "helper.EXE"), "x", 0o644)

	first, err := f.place(req, models.Candidate{Name: "tool", Target: filepath.Join(bin, "tool.exe")})
	require.NoError(t, err)
	second, err := f.place(req, models.Candidate{Name: "renamed", Target: filepath.Join(bin, "helper.EXE"), Declared: true})
	require.NoError(t, err)

	assert.Equal(t, models.Placement{Location: bin}, first)
	assert.Equal(t, models.Placement{Location: bin}, second)
	assert.Equal(t, []string{`C:\Windows`, `%USERPROFILE%\tools`, bin}, f.entries())
	assert.Equal(t, 1, f.UserPath.Writes)
	assert.Equal(t, 1, f.UserPath.Broadcasts)
}

func TestPathDir_Place_Refusals(t *testing.T) {
	f := newPathDirFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "dir.exe"), 0o750))
	mocks.WriteFile(t, filepath.Join(wd, "tool.bat"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(wd, "a;b", "tool.exe"), "x", 0o644)
	mocks.WriteFile(t, filepath.Join(wd, "tool.quiver-new.exe"), "x", 0o644)
	outside := filepath.Join(t.TempDir(), "tool.exe")
	mocks.WriteFile(t, outside, "x", 0o644)

	testCases := []struct {
		name   string
		target string
		want   string
	}{
		{name: "path separator in the folder", target: filepath.Join(wd, "a;b", "tool.exe"), want: models.ReasonUnsafePath},
		{name: "expandable folder", target: filepath.Join(wd, "%TEMP%", "tool.exe"), want: models.ReasonUnsafePath},
		{name: "missing target", target: filepath.Join(wd, "missing.exe"), want: models.ReasonNotFound},
		{name: "directory target", target: filepath.Join(wd, "dir.exe"), want: models.ReasonWrongType},
		{name: "not an exe", target: filepath.Join(wd, "tool.bat"), want: models.ReasonWrongType},
		{name: "outside the workdir", target: outside, want: models.ReasonOutsideWorkdir},
		{name: "unsafe command name", target: filepath.Join(wd, "tool.quiver-new.exe"), want: models.ReasonUnsafeName},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.place(req, models.Candidate{Name: "tool", Target: tc.target})

			require.NoError(t, err)
			assert.Equal(t, models.Placement{Refused: tc.want}, got)
			assert.Zero(t, f.UserPath.Writes)
		})
	}
}

func TestPathDir_Place_WriteError(t *testing.T) {
	boom := errors.New("boom")
	f := newPathDirFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	target := filepath.Join(wd, "tool.exe")
	mocks.WriteFile(t, target, "x", 0o644)

	f.UserPath.WriteErr = boom
	_, err := f.place(req, models.Candidate{Name: "tool", Target: target})
	require.ErrorIs(t, err, boom)
}

func TestPathDir_Remove(t *testing.T) {
	f := newPathDirFixture(t)
	v1 := f.Workdir(t, mocks.NsA)
	v2 := f.Workdir(t, mocks.NsA2)
	other := f.Workdir(t, mocks.NsB)
	vanished := filepath.Join(f.NsDir, "github.com", "acme", "tool@v0", "bin")
	user := `C:\Users\me\tools`
	kept := filepath.Join(v2, "kept")
	require.NoError(t, os.MkdirAll(kept, 0o750))
	testCases := []struct {
		name  string
		claim models.Claim
		keep  map[string]bool
		want  []string
	}{
		{
			name:  "a namespace claim takes every entry of the namespace but those kept",
			claim: models.NamespaceClaim(mocks.BareA),
			keep:  map[string]bool{strings.ToUpper(kept) + string(filepath.Separator): true},
			want:  []string{user, "", kept, other, "%Q%"},
		},
		{
			name:  "a workdir claim spares a sibling ref's live workdir",
			claim: models.WorkdirClaim(mocks.BareA, v1),
			want:  []string{user, "", v2, kept, other, "%Q%"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f.UserPath.Value = strings.Join([]string{user, v1, "", v2, kept, vanished, other, "%Q%"}, userpath.Separator)
			f.UserPath.Writes = 0

			require.NoError(t, f.exposer.Remove(context.Background(), f.Layout(t), tc.claim, tc.keep))

			assert.Equal(t, tc.want, f.entries())
			assert.Equal(t, 1, f.UserPath.Writes)
		})
	}
}

func TestPathDir_Remove_NothingOwnedWritesNothing(t *testing.T) {
	f := newPathDirFixture(t)
	f.UserPath.Value = `C:\Windows;C:\Users\me\tools`

	require.NoError(t, f.exposer.Remove(context.Background(), f.Layout(t), models.NamespaceClaim(mocks.BareA), nil))

	assert.Equal(t, `C:\Windows;C:\Users\me\tools`, f.UserPath.Value)
	assert.Zero(t, f.UserPath.Writes)
	assert.Zero(t, f.UserPath.Broadcasts)
}

func TestPathDir_Remove_ReadError(t *testing.T) {
	boom := errors.New("boom")
	f := newPathDirFixture(t)
	f.UserPath.ReadErr = boom

	err := f.exposer.Remove(context.Background(), f.Layout(t), models.NamespaceClaim(mocks.BareA), nil)

	require.ErrorIs(t, err, boom)
}
