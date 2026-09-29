package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

func TestShimContent(t *testing.T) {
	got := shimContent(mocks.BareA, `C:\q\namespaces\github.com\acme\tool@v1`, `C:\q\namespaces\github.com\acme\tool@v1\rg.exe`)

	assert.Equal(t, "@echo off\r\nREM quiver:github.com/acme/tool\r\nREM quiver-workdir:C:\\q\\namespaces\\github.com\\acme\\tool@v1\r\n\"C:\\q\\namespaces\\github.com\\acme\\tool@v1\\rg.exe\" %*\r\n", got)
}

func TestPlaceShim(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "rg.exe")
	mocks.WriteFile(t, target, "x", 0o755)
	percent := filepath.Join(wd, "100%.exe")
	mocks.WriteFile(t, percent, "x", 0o755)
	bang := filepath.Join(wd, "bang!.exe")
	mocks.WriteFile(t, bang, "x", 0o755)
	l := f.Layout(t)
	req := models.ApplyRequest{Layout: l, Bare: mocks.BareA, Workdir: wd}
	loc := filepath.Join(f.Bin, "rg.cmd")

	testCases := []struct {
		name     string
		existing string
		target   string
		want     models.Placement
	}{
		{name: "new shim", target: target, want: models.Placement{Location: loc}},
		{name: "own shim", existing: shimContent(mocks.BareA, "wd", "old"), target: target, want: models.Placement{Location: loc}},
		{name: "foreign shim", existing: shimContent(mocks.BareB, "wd", "old"), target: target, want: models.Placement{Refused: "owned by github.com/other/thing"}},
		{name: "user file", existing: "@echo off\r\n", target: target, want: models.Placement{Refused: models.ReasonUnmanaged}},
		{name: "unsafe target", target: percent, want: models.Placement{Refused: models.ReasonUnsafePath}},
		{name: "delayed expansion target", target: bang, want: models.Placement{Refused: models.ReasonUnsafePath}},
		{name: "missing target", target: filepath.Join(wd, "missing.exe"), want: models.Placement{Refused: models.ReasonNotFound}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.RemoveAll(loc))
			if tc.existing != "" {
				mocks.WriteFile(t, loc, tc.existing, 0o600)
			}

			got, err := placeShim(req, models.Candidate{Name: "rg", Target: tc.target})

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			if tc.want.Location == "" {
				return
			}
			data, err := os.ReadFile(loc)
			require.NoError(t, err)
			assert.Equal(t, shimContent(mocks.BareA, wd, target), string(data))
		})
	}
}

func TestPlaceShim_Errors(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "rg.exe")
	mocks.WriteFile(t, target, "x", 0o755)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := placeShim(models.ApplyRequest{Layout: platform.Layout{Bin: file}, Bare: mocks.BareA, Workdir: wd}, models.Candidate{Name: "rg", Target: target})

	require.Error(t, err)
}

func TestRemoveShims(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	own := filepath.Join(f.Bin, "own.cmd")
	foreign := filepath.Join(f.Bin, "foreign.cmd")
	plain := filepath.Join(f.Bin, "plain.cmd")
	other := filepath.Join(f.Bin, "notes.txt")
	mocks.WriteFile(t, own, shimContent(mocks.BareA, "wd", "x"), 0o600)
	mocks.WriteFile(t, foreign, shimContent(mocks.BareB, "wd", "x"), 0o600)
	mocks.WriteFile(t, plain, "@echo off\r\n", 0o600)
	mocks.WriteFile(t, other, shimContent(mocks.BareA, "wd", "x"), 0o600)
	require.NoError(t, os.MkdirAll(filepath.Join(f.Bin, "dir.cmd"), 0o750))
	kept := filepath.Join(f.Bin, "kept.cmd")
	mocks.WriteFile(t, kept, shimContent(mocks.BareA, "wd", "x"), 0o600)

	require.NoError(t, removeShims(platform.Layout{Bin: f.Bin}, ownership.NamespaceClaim(mocks.BareA), map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	assert.FileExists(t, kept)
	assert.FileExists(t, foreign)
	assert.FileExists(t, plain)
	assert.FileExists(t, other)
}

func TestRemoveShims_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.NoError(t, removeShims(platform.Layout{Bin: filepath.Join(t.TempDir(), "missing")}, ownership.NamespaceClaim(mocks.BareA), nil))
	assert.Error(t, removeShims(platform.Layout{Bin: file}, ownership.NamespaceClaim(mocks.BareA), nil))

	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	bin := t.TempDir()
	unreadable := filepath.Join(bin, "x.cmd")
	mocks.WriteFile(t, unreadable, shimContent(mocks.BareA, "wd", "x"), 0o000)

	assert.Error(t, removeShims(platform.Layout{Bin: bin}, ownership.NamespaceClaim(mocks.BareA), nil))

	require.NoError(t, os.Chmod(unreadable, 0o600))
	require.NoError(t, os.Chmod(bin, 0o500))
	t.Cleanup(func() { _ = os.Chmod(bin, 0o750) })

	assert.Error(t, removeShims(platform.Layout{Bin: bin}, ownership.NamespaceClaim(mocks.BareA), nil))
}

func TestPlaceShim_SwapError(t *testing.T) {
	f := newFixture(t, platform.GOOSWindows)
	wd := f.Workdir(t, mocks.NsA)
	target := filepath.Join(wd, "rg.exe")
	mocks.WriteFile(t, target, "x", 0o755)
	mocks.WriteFile(t, filepath.Join(f.Bin, "rg.cmd"+fsguard.StagedSuffix, "x"), "", 0o600)
	req := models.ApplyRequest{Layout: f.Layout(t), Bare: mocks.BareA, Workdir: wd}

	_, err := placeShim(req, models.Candidate{Name: "rg", Target: target})

	require.Error(t, err)
}
