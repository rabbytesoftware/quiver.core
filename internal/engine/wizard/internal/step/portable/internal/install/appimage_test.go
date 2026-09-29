package install_test

import (
	"bytes"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/dest"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestInstall_AppImageWithoutIconRecordsNoIcon(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteAppImage(t, workDir, "tool", map[string]mocks.Entry{
		"AppRun": {Mode: 0o755, Data: "#!/bin/sh\n"},
	})

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "tool", "apps", "")

	require.NoError(t, err)
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{{
		Name:  "tool.AppDir",
		Entry: "apps/tool.AppDir/.quiver-run",
	}}}, readRecord(t, workDir))
}

func TestInstall_AppImageType1Unsupported(t *testing.T) {
	workDir := t.TempDir()
	built := mocks.BuildAppImage(t, map[string]mocks.Entry{"AppRun": {Data: "x"}}, 1)
	data, err := os.ReadFile(built)
	require.NoError(t, err)
	from := mocks.WriteFile(t, filepath.Join(workDir, "old.AppImage"), data)

	err = runPortable(t, wizstep.Request{WorkDir: workDir}, "old.AppImage", ".", "")

	require.Error(t, err)
	assert.FileExists(t, from)
}

func TestInstall_AppImageFailures(t *testing.T) {
	testCases := []struct {
		name    string
		entries map[string]mocks.Entry
		to      string
		setup   func(t *testing.T, workDir string)
		check   func(t *testing.T, workDir string)
	}{
		{
			name:    "destination is a file",
			entries: mocks.AppImageEntries("bruno"),
			to:      "blocker",
			setup: func(t *testing.T, workDir string) {
				mocks.WriteFile(t, filepath.Join(workDir, "blocker"), []byte("x"))
			},
			check: func(t *testing.T, workDir string) {
				assert.Equal(t, "x", mocks.ReadString(t, filepath.Join(workDir, "blocker")))
			},
		},
		{
			name: "launcher location is a directory",
			entries: map[string]mocks.Entry{
				"AppRun":                           {Mode: 0o755, Data: "#!/bin/sh\n"},
				unpack.LauncherName + "/blocker":   {Mode: 0o644, Data: "x"},
				unpack.LauncherName + "/blocker-2": {Mode: 0o644, Data: "x"},
			},
			to:    ".",
			setup: func(t *testing.T, workDir string) {},
			check: func(t *testing.T, workDir string) {
				assert.NoDirExists(t, filepath.Join(workDir, "bruno"))
				assert.NoDirExists(t, filepath.Join(workDir, ".bruno.quiver-tmp"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			mocks.WriteAppImage(t, workDir, "bruno.AppImage", tc.entries)
			tc.setup(t, workDir)

			err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", tc.to, "")

			require.Error(t, err)
			tc.check(t, workDir)
			assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
		})
	}
}

func TestInstall_AppImageUnreadableDesktopFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not block reads here")
	}

	const markerPerm = 0o604
	workDir := t.TempDir()
	built := mocks.BuildAppImage(t, map[string]mocks.Entry{
		"AppRun":        {Mode: 0o755, Data: "#!/bin/sh\n"},
		"bruno.desktop": {Mode: markerPerm, Data: "[Desktop Entry]\nName=Bruno\n"},
	}, 2)
	data, err := os.ReadFile(built)
	require.NoError(t, err)
	fileInode := binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint16(nil, 2), markerPerm)
	require.Equal(t, 1, bytes.Count(data, fileInode))
	unreadable := binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint16(nil, 2), 0o200)
	mocks.WriteFile(t, filepath.Join(workDir, "bruno.AppImage"), bytes.Replace(data, fileInode, unreadable, 1))

	err = runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrPermission)
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

func TestInstall_AppImageReinstallRemovesStaleFiles(t *testing.T) {
	workDir := t.TempDir()
	req := wizstep.Request{WorkDir: workDir}
	old := mocks.AppImageEntries("bruno")
	old["usr/lib/libold.so"] = mocks.Entry{Mode: 0o644, Data: "old"}
	mocks.WriteAppImage(t, workDir, "bruno.AppImage", old)
	require.NoError(t, runPortable(t, req, "bruno.AppImage", ".", ""))
	require.FileExists(t, filepath.Join(workDir, "bruno", "usr", "lib", "libold.so"))

	next := mocks.AppImageEntries("bruno")
	next["usr/lib/libnew.so"] = mocks.Entry{Mode: 0o644, Data: "new"}
	mocks.WriteAppImage(t, workDir, "bruno.AppImage", next)
	require.NoError(t, runPortable(t, req, "bruno.AppImage", ".", ""))

	assert.NoFileExists(t, filepath.Join(workDir, "bruno", "usr", "lib", "libold.so"))
	assert.Equal(t, "new", mocks.ReadString(t, filepath.Join(workDir, "bruno", "usr", "lib", "libnew.so")))
	assert.FileExists(t, filepath.Join(workDir, "bruno", unpack.LauncherName))
}

func TestInstall_AppImageCraftedNamesStayInsideDestination(t *testing.T) {
	testCases := []struct {
		name       string
		file       string
		wantAppDir string
	}{
		{name: "dot stem", file: "..AppImage", wantAppDir: "..AppImage.AppDir"},
		{name: "dot-dot stem", file: "...AppImage", wantAppDir: "...AppImage.AppDir"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			workDir := filepath.Join(parent, "work")
			to := filepath.Join(workDir, "apps")
			parentSentinel := mocks.WriteFile(t, filepath.Join(workDir, "keep"), []byte("x"))
			toSentinel := mocks.WriteFile(t, filepath.Join(to, "keep"), []byte("x"))
			grandSentinel := mocks.WriteFile(t, filepath.Join(parent, "keep"), []byte("x"))
			mocks.WriteAppImage(t, workDir, tc.file, mocks.AppImageEntries("bruno"))

			err := runPortable(t, wizstep.Request{WorkDir: workDir}, tc.file, "apps", "")

			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(to, tc.wantAppDir, unpack.LauncherName))
			assert.FileExists(t, parentSentinel)
			assert.FileExists(t, toSentinel)
			assert.FileExists(t, grandSentinel)
		})
	}
}

func TestInstall_AppImageCorruptKeepsExistingAppDir(t *testing.T) {
	workDir := t.TempDir()
	req := wizstep.Request{WorkDir: workDir}
	mocks.WriteAppImage(t, workDir, "bruno.AppImage", mocks.AppImageEntries("bruno"))
	require.NoError(t, runPortable(t, req, "bruno.AppImage", ".", ""))
	launcher := filepath.Join(workDir, "bruno", unpack.LauncherName)
	before := mocks.ReadString(t, launcher)

	built := mocks.BuildAppImage(t, mocks.AppImageEntries("bruno"), 2)
	data, err := os.ReadFile(built)
	require.NoError(t, err)
	mocks.WriteFile(t, filepath.Join(workDir, "bruno.AppImage"), data[:len(data)-len(data)/4])

	err = runPortable(t, req, "bruno.AppImage", ".", "")

	require.Error(t, err)
	assert.Equal(t, before, mocks.ReadString(t, launcher))
	assert.FileExists(t, filepath.Join(workDir, "bruno", "AppRun"))
	assert.NoDirExists(t, filepath.Join(workDir, ".bruno.quiver-tmp"))
}

func TestInstall_AppImageUnownedAppDirUntouched(t *testing.T) {
	workDir := t.TempDir()
	keep := mocks.WriteFile(t, filepath.Join(workDir, "bruno", "notes.txt"), []byte("mine"))
	mocks.WriteAppImage(t, workDir, "bruno.AppImage", mocks.AppImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.ErrorIs(t, err, dest.ErrUnowned)
	assert.Equal(t, "mine", mocks.ReadString(t, keep))
	assert.NoFileExists(t, filepath.Join(workDir, "bruno", unpack.LauncherName))
	assert.NoDirExists(t, filepath.Join(workDir, ".bruno.quiver-tmp"))
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

func TestInstall_AppImageCleansLeftoverStaging(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, ".bruno.quiver-tmp", "usr", "lib", "crashed.so"), []byte("x"))
	mocks.WriteAppImage(t, workDir, "bruno.AppImage", mocks.AppImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(workDir, ".bruno.quiver-tmp"))
	assert.NoFileExists(t, filepath.Join(workDir, "bruno", "usr", "lib", "crashed.so"))
	assert.FileExists(t, filepath.Join(workDir, "bruno", unpack.LauncherName))
}
