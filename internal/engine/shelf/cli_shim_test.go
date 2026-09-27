package shelf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShimContent(t *testing.T) {
	got := shimContent(bareA, `C:\q\namespaces\github.com\acme\tool@v1\rg.exe`)

	assert.Equal(t, "@echo off\r\nREM quiver:github.com/acme/tool\r\n\"C:\\q\\namespaces\\github.com\\acme\\tool@v1\\rg.exe\" %*\r\n", got)
}

func TestPlaceShim(t *testing.T) {
	f := newFixture(t, goosWindows)
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "rg.exe")
	writeFile(t, target, "x", 0o755)
	percent := filepath.Join(wd, "100%.exe")
	writeFile(t, percent, "x", 0o755)
	bang := filepath.Join(wd, "bang!.exe")
	writeFile(t, bang, "x", 0o755)
	l, err := f.shelf.layout()
	require.NoError(t, err)
	req := applyRequest{layout: l, bare: bareA, workdir: wd}
	loc := filepath.Join(f.bin, "rg.cmd")

	testCases := []struct {
		name     string
		existing string
		target   string
		want     placement
	}{
		{name: "new shim", target: target, want: placement{location: loc}},
		{name: "own shim", existing: shimContent(bareA, "old"), target: target, want: placement{location: loc}},
		{name: "foreign shim", existing: shimContent(bareB, "old"), target: target, want: placement{refused: "owned by github.com/other/thing"}},
		{name: "user file", existing: "@echo off\r\n", target: target, want: placement{refused: reasonUnmanaged}},
		{name: "unsafe target", target: percent, want: placement{refused: reasonUnsafePath}},
		{name: "delayed expansion target", target: bang, want: placement{refused: reasonUnsafePath}},
		{name: "missing target", target: filepath.Join(wd, "missing.exe"), want: placement{refused: reasonNotFound}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.RemoveAll(loc))
			if tc.existing != "" {
				writeFile(t, loc, tc.existing, 0o600)
			}

			got, err := placeShim(req, candidate{name: "rg", target: tc.target})

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			if tc.want.location == "" {
				return
			}
			data, err := os.ReadFile(loc)
			require.NoError(t, err)
			assert.Equal(t, shimContent(bareA, target), string(data))
		})
	}
}

func TestPlaceShim_Errors(t *testing.T) {
	f := newFixture(t, goosWindows)
	wd := f.workdir(t, nsA)
	target := filepath.Join(wd, "rg.exe")
	writeFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := placeShim(applyRequest{layout: layout{bin: file}, bare: bareA, workdir: wd}, candidate{name: "rg", target: target})

	require.Error(t, err)
}

func TestRemoveShims(t *testing.T) {
	f := newFixture(t, goosWindows)
	own := filepath.Join(f.bin, "own.cmd")
	foreign := filepath.Join(f.bin, "foreign.cmd")
	plain := filepath.Join(f.bin, "plain.cmd")
	other := filepath.Join(f.bin, "notes.txt")
	writeFile(t, own, shimContent(bareA, "x"), 0o600)
	writeFile(t, foreign, shimContent(bareB, "x"), 0o600)
	writeFile(t, plain, "@echo off\r\n", 0o600)
	writeFile(t, other, shimContent(bareA, "x"), 0o600)
	require.NoError(t, os.MkdirAll(filepath.Join(f.bin, "dir.cmd"), 0o750))
	kept := filepath.Join(f.bin, "kept.cmd")
	writeFile(t, kept, shimContent(bareA, "x"), 0o600)

	require.NoError(t, removeShims(layout{bin: f.bin}, bareA, map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	assert.FileExists(t, kept)
	assert.FileExists(t, foreign)
	assert.FileExists(t, plain)
	assert.FileExists(t, other)
}

func TestRemoveShims_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.NoError(t, removeShims(layout{bin: filepath.Join(t.TempDir(), "missing")}, bareA, nil))
	assert.Error(t, removeShims(layout{bin: file}, bareA, nil))

	requireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	bin := t.TempDir()
	unreadable := filepath.Join(bin, "x.cmd")
	writeFile(t, unreadable, shimContent(bareA, "x"), 0o000)

	assert.Error(t, removeShims(layout{bin: bin}, bareA, nil))

	require.NoError(t, os.Chmod(unreadable, 0o600))
	require.NoError(t, os.Chmod(bin, 0o500))
	t.Cleanup(func() { _ = os.Chmod(bin, 0o750) })

	assert.Error(t, removeShims(layout{bin: bin}, bareA, nil))
}
