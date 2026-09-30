//go:build windows

package lnk

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shell runs a PowerShell script against WScript.Shell, passing values through
// the environment so no path is ever spliced into the script.
func shell(
	t *testing.T,
	script string,
	env ...string,
) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8;"+script)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.Split(strings.TrimRight(string(out), "\r\n"), "\r\n")
}

func longTempDir(
	t *testing.T,
) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func samePath(
	t *testing.T,
	want string,
	got string,
) {
	t.Helper()
	require.NotEmpty(t, got, "want %s", want)
	resolved, err := filepath.EvalSymlinks(got)
	require.NoError(t, err, got)
	assert.True(t, strings.EqualFold(want, resolved), "want %s, got %s", want, got)
}

func TestShellLink_WindowsReadsWhatEncodeWrites(t *testing.T) {
	for _, folder := range []string{"app", "Café app"} {
		t.Run(folder, func(t *testing.T) {
			dir := longTempDir(t)
			target := filepath.Join(dir, folder, "Tool.exe")
			require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o750))
			require.NoError(t, os.WriteFile(target, []byte("MZ"), 0o600))
			icon := filepath.Join(dir, folder, "tool.ico")
			require.NoError(t, os.WriteFile(icon, nil, 0o600))
			loc := filepath.Join(dir, "Tool.lnk")
			data, err := Encode(Link{
				Target:      target,
				WorkingDir:  filepath.Dir(target),
				Description: `quiver:github.com/acme/tool|` + dir,
				Icon:        icon,
			})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(loc, data, 0o600))

			got := shell(t, "$s=(New-Object -ComObject WScript.Shell).CreateShortcut($env:QUIVER_LNK);"+
				"$s.TargetPath;$s.Description;$s.WorkingDirectory;$s.IconLocation",
				"QUIVER_LNK="+loc)

			require.Len(t, got, 4)
			samePath(t, target, got[0])
			assert.Equal(t, `quiver:github.com/acme/tool|`+dir, got[1])
			samePath(t, filepath.Dir(target), got[2])
			assert.True(t, strings.EqualFold(icon+",0", got[3]), got[3])
		})
	}
}

func TestShellLink_DecodeReadsWhatWindowsWrites(t *testing.T) {
	dir := longTempDir(t)
	target := filepath.Join(dir, "app", "Tool.exe")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o750))
	require.NoError(t, os.WriteFile(target, []byte("MZ"), 0o600))
	loc := filepath.Join(dir, "Tool.lnk")
	description := `quiver:github.com/acme/tool|` + dir

	shell(t, "$s=(New-Object -ComObject WScript.Shell).CreateShortcut($env:QUIVER_LNK);"+
		"$s.TargetPath=$env:QUIVER_TARGET;$s.WorkingDirectory=(Split-Path -Parent $env:QUIVER_TARGET);"+
		"$s.Description=$env:QUIVER_DESC;$s.Save()",
		"QUIVER_LNK="+loc, "QUIVER_TARGET="+target, "QUIVER_DESC="+description)

	data, err := os.ReadFile(loc) // #nosec G304 -- test reads the shortcut PowerShell just wrote
	require.NoError(t, err)
	got, err := Decode(data)
	require.NoError(t, err)

	assert.Equal(t, description, got.Description)
	samePath(t, target, got.Target)
	samePath(t, filepath.Dir(target), got.WorkingDir)
	h, err := holder(loc)
	require.NoError(t, err)
	assert.Equal(t, "github.com/acme/tool", h.Namespace.String())
	assert.Equal(t, dir, h.Target)
}
