package desktop

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
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

func describeResponder(
	description string,
	describeErr error,
	createErr error,
) func(string, []string, []string) ([]byte, error) {
	return func(_ string, args, _ []string) ([]byte, error) {
		if args[len(args)-1] == lnkDescribeScript {
			return []byte(description), describeErr
		}
		return nil, createErr
	}
}

func TestPlacer_PlaceLnk(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool.exe")
	mocks.WriteFile(t, target, "x", 0o755)
	l := f.Layout(t)
	req := models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}
	loc := filepath.Join(lnkDir(filepath.Join(f.UserHome, "AppData", "Roaming")), "Tool.lnk")
	boom := errors.New("boom")

	testCases := []struct {
		name        string
		existing    bool
		description string
		describeErr error
		createErr   error
		target      string
		want        models.Placement
		wantCalls   int
		wantErr     bool
	}{
		{name: "new shortcut", target: target, want: models.Placement{Location: loc}, wantCalls: 1},
		{name: "own shortcut", existing: true, description: "quiver:github.com/acme/tool\r\n", target: target, want: models.Placement{Location: loc}, wantCalls: 2},
		{name: "foreign shortcut", existing: true, description: "quiver:github.com/other/thing", target: target, want: models.Placement{Refused: "owned by github.com/other/thing"}, wantCalls: 1},
		{name: "user shortcut", existing: true, description: "Some App", target: target, want: models.Placement{Refused: models.ReasonUnmanaged}, wantCalls: 1},
		{name: "missing target", target: filepath.Join(wd, "missing.exe"), want: models.Placement{Refused: models.ReasonNotFound}},
		{name: "describe fails", existing: true, describeErr: boom, target: target, wantErr: true, wantCalls: 1},
		{name: "create fails", createErr: boom, target: target, wantErr: true, wantCalls: 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.RemoveAll(loc))
			if tc.existing {
				mocks.WriteFile(t, loc, "lnk", 0o600)
			}
			f.Cmd.Calls = nil
			f.Cmd.Respond = describeResponder(tc.description, tc.describeErr, tc.createErr)

			got, err := f.placer.placeLnk(context.Background(), req, domain.ExposeEntry{}, models.Candidate{Name: "Tool", Target: tc.target})

			assert.Len(t, f.Cmd.Calls, tc.wantCalls)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPlacer_PlaceLnk_PassesValuesThroughTheEnvironment(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "it's.exe")
	mocks.WriteFile(t, target, "x", 0o755)
	l := f.Layout(t)
	loc := filepath.Join(lnkDir(filepath.Join(f.UserHome, "AppData", "Roaming")), "Tool.lnk")
	req := models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}

	testCases := []struct {
		name     string
		icon     string
		wantIcon string
	}{
		{name: "ico icon", icon: "${INSTALL_PATH}/tool.ico", wantIcon: filepath.Join(wd, "tool.ico")},
		{name: "png icon is dropped", icon: "${INSTALL_PATH}/tool.png", wantIcon: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f.Cmd.Calls = nil
			f.Cmd.Envs = nil

			_, err := f.placer.placeLnk(context.Background(), req, domain.ExposeEntry{Icon: tc.icon}, models.Candidate{Name: "Tool", Target: target})
			require.NoError(t, err)

			require.Len(t, f.Cmd.Calls, 1)
			assert.Equal(t, []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", lnkCreateScript}, f.Cmd.Calls[0])
			assert.Equal(t, []string{
				"QUIVER_LNK_PATH=" + loc,
				"QUIVER_LNK_TARGET=" + target,
				"QUIVER_LNK_ICON=" + tc.wantIcon,
				"QUIVER_LNK_DESC=quiver:github.com/acme/tool|" + wd,
			}, f.Cmd.Envs[0])
			assert.NotContains(t, lnkCreateScript, "it's")
		})
	}
}

func TestPlacer_LnkHolder_PassesPathThroughTheEnvironment(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	loc := filepath.Join(t.TempDir(), "Tool.lnk")
	mocks.WriteFile(t, loc, "", 0o600)
	f.Cmd.Respond = describeResponder("quiver:github.com/acme/tool|C:\\wd", nil, nil)

	got, err := f.placer.lnkHolder(context.Background(), loc)

	require.NoError(t, err)
	assert.Equal(t, ownership.Holder{Exists: true, Namespace: mocks.BareA, Target: `C:\wd`}, got)
	assert.Equal(t, [][]string{{"QUIVER_LNK_PATH=" + loc}}, f.Cmd.Envs)
	assert.Equal(t, lnkDescribeScript, f.Cmd.Calls[0][len(f.Cmd.Calls[0])-1])
}

func TestPlacer_PlaceLnk_MkdirError(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "tool.exe")
	mocks.WriteFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := f.placer.placeLnk(context.Background(), models.ApplyRequest{Layout: platform.Layout{UserHome: file}, Bare: mocks.BareA, Workdir: wd}, domain.ExposeEntry{}, models.Candidate{Name: "Tool", Target: target})

	require.Error(t, err)
}

func TestPlacer_RemoveLnks(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	dir := lnkDir(filepath.Join(f.UserHome, "AppData", "Roaming"))
	own := filepath.Join(dir, "own.lnk")
	foreign := filepath.Join(dir, "foreign.lnk")
	notes := filepath.Join(dir, "notes.txt")
	mocks.WriteFile(t, own, "", 0o600)
	mocks.WriteFile(t, foreign, "", 0o600)
	mocks.WriteFile(t, notes, "", 0o600)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub.lnk"), 0o750))
	kept := filepath.Join(dir, "own-kept.lnk")
	mocks.WriteFile(t, kept, "", 0o600)
	f.Cmd.Respond = func(_ string, _, env []string) ([]byte, error) {
		if strings.HasSuffix(env[0], "own.lnk") {
			return []byte("quiver:github.com/acme/tool"), nil
		}
		return []byte("quiver:github.com/other/thing"), nil
	}

	require.NoError(t, f.placer.removeLnks(context.Background(), platform.Layout{UserHome: f.UserHome}, ownership.NamespaceClaim(mocks.BareA), map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	assert.FileExists(t, kept)
	assert.FileExists(t, foreign)
	assert.FileExists(t, notes)
	assert.Len(t, f.Cmd.Calls, 2)
}

func TestPlacer_RemoveLnks_WorkdirClaimSparesSiblingRef(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	v1 := f.Workdir(t, mocks.NsA)
	v2 := f.Workdir(t, mocks.NsA2)
	dir := lnkDir(filepath.Join(f.UserHome, "AppData", "Roaming"))
	mine := filepath.Join(dir, "mine.lnk")
	sibling := filepath.Join(dir, "sibling.lnk")
	mocks.WriteFile(t, mine, "", 0o600)
	mocks.WriteFile(t, sibling, "", 0o600)
	f.Cmd.Respond = func(_ string, _, env []string) ([]byte, error) {
		if strings.HasSuffix(env[0], "mine.lnk") {
			return []byte(lnkDescription(mocks.BareA, v1)), nil
		}
		return []byte(lnkDescription(mocks.BareA, v2)), nil
	}

	require.NoError(t, f.placer.removeLnks(context.Background(), platform.Layout{UserHome: f.UserHome}, ownership.WorkdirClaim(mocks.BareA, v1), nil))

	assert.NoFileExists(t, mine)
	assert.FileExists(t, sibling)
}

func TestPlacer_RemoveLnks_Errors(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	missing := filepath.Join(t.TempDir(), "missing")
	blocked := t.TempDir()
	mocks.WriteFile(t, lnkDir(filepath.Join(blocked, "AppData", "Roaming")), "", 0o600)
	failing := t.TempDir()
	mocks.WriteFile(t, filepath.Join(lnkDir(filepath.Join(failing, "AppData", "Roaming")), "x.lnk"), "", 0o600)
	f.Cmd.Respond = describeResponder("", errors.New("boom"), nil)

	assert.NoError(t, f.placer.removeLnks(context.Background(), platform.Layout{UserHome: missing}, ownership.NamespaceClaim(mocks.BareA), nil))
	assert.Error(t, f.placer.removeLnks(context.Background(), platform.Layout{UserHome: blocked}, ownership.NamespaceClaim(mocks.BareA), nil))
	assert.Error(t, f.placer.removeLnks(context.Background(), platform.Layout{UserHome: failing}, ownership.NamespaceClaim(mocks.BareA), nil))

	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	home := t.TempDir()
	dir := lnkDir(filepath.Join(home, "AppData", "Roaming"))
	mocks.WriteFile(t, filepath.Join(dir, "x.lnk"), "", 0o600)
	f.Cmd.Respond = describeResponder("quiver:github.com/acme/tool", nil, nil)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	assert.Error(t, f.placer.removeLnks(context.Background(), platform.Layout{UserHome: home}, ownership.NamespaceClaim(mocks.BareA), nil))
}

func TestPlacer_LnkHolder_InspectError(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := f.placer.lnkHolder(context.Background(), filepath.Join(file, "Tool.lnk"))

	require.Error(t, err)
}
