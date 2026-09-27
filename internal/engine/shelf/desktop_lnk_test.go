package shelf

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
)

func TestShelf_AppData(t *testing.T) {
	f := newFixture(t, goosWindows)

	assert.Equal(t, filepath.Join("/home", "AppData", "Roaming"), f.shelf.appData("/home"))
	f.env["APPDATA"] = `C:\Roaming`
	assert.Equal(t, `C:\Roaming`, f.shelf.appData("/home"))
}

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

func TestShelf_PlaceLnk(t *testing.T) {
	f := newFixture(t, goosWindows)
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool.exe")
	writeFile(t, target, "x", 0o755)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	req := applyRequest{layout: l, bare: bareA, workdir: wd}
	loc := filepath.Join(lnkDir(filepath.Join(f.userHome, "AppData", "Roaming")), "Tool.lnk")
	boom := errors.New("boom")

	testCases := []struct {
		name        string
		existing    bool
		description string
		describeErr error
		createErr   error
		target      string
		want        placement
		wantCalls   int
		wantErr     bool
	}{
		{name: "new shortcut", target: target, want: placement{location: loc}, wantCalls: 1},
		{name: "own shortcut", existing: true, description: "quiver:github.com/acme/tool\r\n", target: target, want: placement{location: loc}, wantCalls: 2},
		{name: "foreign shortcut", existing: true, description: "quiver:github.com/other/thing", target: target, want: placement{refused: "owned by github.com/other/thing"}, wantCalls: 1},
		{name: "user shortcut", existing: true, description: "Some App", target: target, want: placement{refused: reasonUnmanaged}, wantCalls: 1},
		{name: "missing target", target: filepath.Join(wd, "missing.exe"), want: placement{refused: reasonNotFound}},
		{name: "describe fails", existing: true, describeErr: boom, target: target, wantErr: true, wantCalls: 1},
		{name: "create fails", createErr: boom, target: target, wantErr: true, wantCalls: 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.RemoveAll(loc))
			if tc.existing {
				writeFile(t, loc, "lnk", 0o600)
			}
			f.cmd.calls = nil
			f.cmd.respond = describeResponder(tc.description, tc.describeErr, tc.createErr)

			got, err := f.shelf.placeLnk(context.Background(), req, domain.ExposeEntry{}, candidate{name: "Tool", target: tc.target})

			assert.Len(t, f.cmd.calls, tc.wantCalls)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestShelf_PlaceLnk_PassesValuesThroughTheEnvironment(t *testing.T) {
	f := newFixture(t, goosWindows)
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "it's.exe")
	writeFile(t, target, "x", 0o755)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	loc := filepath.Join(lnkDir(filepath.Join(f.userHome, "AppData", "Roaming")), "Tool.lnk")
	req := applyRequest{layout: l, bare: bareA, workdir: wd}

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
			f.cmd.calls = nil
			f.cmd.envs = nil

			_, err := f.shelf.placeLnk(context.Background(), req, domain.ExposeEntry{Icon: tc.icon}, candidate{name: "Tool", target: target})
			require.NoError(t, err)

			require.Len(t, f.cmd.calls, 1)
			assert.Equal(t, []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", lnkCreateScript}, f.cmd.calls[0])
			assert.Equal(t, []string{
				"QUIVER_LNK_PATH=" + loc,
				"QUIVER_LNK_TARGET=" + target,
				"QUIVER_LNK_ICON=" + tc.wantIcon,
				"QUIVER_LNK_DESC=quiver:github.com/acme/tool",
			}, f.cmd.envs[0])
			assert.NotContains(t, lnkCreateScript, "it's")
		})
	}
}

func TestShelf_LnkHolder_PassesPathThroughTheEnvironment(t *testing.T) {
	f := newFixture(t, goosWindows)
	loc := filepath.Join(t.TempDir(), "Tool.lnk")
	writeFile(t, loc, "", 0o600)
	f.cmd.respond = describeResponder("quiver:github.com/acme/tool", nil, nil)

	got, err := f.shelf.lnkHolder(context.Background(), loc)

	require.NoError(t, err)
	assert.Equal(t, holder{exists: true, namespace: bareA}, got)
	assert.Equal(t, [][]string{{"QUIVER_LNK_PATH=" + loc}}, f.cmd.envs)
	assert.Equal(t, lnkDescribeScript, f.cmd.calls[0][len(f.cmd.calls[0])-1])
}

func TestShelf_PlaceLnk_MkdirError(t *testing.T) {
	f := newFixture(t, goosWindows)
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "tool.exe")
	writeFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := f.shelf.placeLnk(context.Background(), applyRequest{layout: layout{userHome: file}, bare: bareA, workdir: wd}, domain.ExposeEntry{}, candidate{name: "Tool", target: target})

	require.Error(t, err)
}

func TestShelf_RemoveLnks(t *testing.T) {
	f := newFixture(t, goosWindows)
	dir := lnkDir(filepath.Join(f.userHome, "AppData", "Roaming"))
	own := filepath.Join(dir, "own.lnk")
	foreign := filepath.Join(dir, "foreign.lnk")
	notes := filepath.Join(dir, "notes.txt")
	writeFile(t, own, "", 0o600)
	writeFile(t, foreign, "", 0o600)
	writeFile(t, notes, "", 0o600)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub.lnk"), 0o750))
	kept := filepath.Join(dir, "own-kept.lnk")
	writeFile(t, kept, "", 0o600)
	f.cmd.respond = func(_ string, _, env []string) ([]byte, error) {
		if strings.HasSuffix(env[0], "own.lnk") {
			return []byte("quiver:github.com/acme/tool"), nil
		}
		return []byte("quiver:github.com/other/thing"), nil
	}

	require.NoError(t, f.shelf.removeLnks(context.Background(), layout{userHome: f.userHome}, bareA, map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	assert.FileExists(t, kept)
	assert.FileExists(t, foreign)
	assert.FileExists(t, notes)
	assert.Len(t, f.cmd.calls, 2)
}

func TestShelf_RemoveLnks_Errors(t *testing.T) {
	f := newFixture(t, goosWindows)
	missing := filepath.Join(t.TempDir(), "missing")
	blocked := t.TempDir()
	writeFile(t, lnkDir(filepath.Join(blocked, "AppData", "Roaming")), "", 0o600)
	failing := t.TempDir()
	writeFile(t, filepath.Join(lnkDir(filepath.Join(failing, "AppData", "Roaming")), "x.lnk"), "", 0o600)
	f.cmd.respond = describeResponder("", errors.New("boom"), nil)

	assert.NoError(t, f.shelf.removeLnks(context.Background(), layout{userHome: missing}, bareA, nil))
	assert.Error(t, f.shelf.removeLnks(context.Background(), layout{userHome: blocked}, bareA, nil))
	assert.Error(t, f.shelf.removeLnks(context.Background(), layout{userHome: failing}, bareA, nil))

	requireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	home := t.TempDir()
	dir := lnkDir(filepath.Join(home, "AppData", "Roaming"))
	writeFile(t, filepath.Join(dir, "x.lnk"), "", 0o600)
	f.cmd.respond = describeResponder("quiver:github.com/acme/tool", nil, nil)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	assert.Error(t, f.shelf.removeLnks(context.Background(), layout{userHome: home}, bareA, nil))
}
