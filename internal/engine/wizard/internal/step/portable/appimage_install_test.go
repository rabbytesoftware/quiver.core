package portable_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func TestHandler_Execute_AppImageWithoutIconRecordsNoIcon(t *testing.T) {
	workDir := t.TempDir()
	writeAppImage(t, workDir, "tool", map[string]unpacktest.Entry{
		"AppRun": {Mode: 0o755, Data: "#!/bin/sh\n"},
	})

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "tool", "apps", "")

	require.NoError(t, err)
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{{
		Name:  "tool.AppDir",
		Entry: "apps/tool.AppDir/.quiver-run",
	}}}, readRecord(t, workDir))
}

func TestHandler_Execute_AppImageType1Unsupported(t *testing.T) {
	workDir := t.TempDir()
	built := unpacktest.BuildAppImage(t, map[string]unpacktest.Entry{"AppRun": {Data: "x"}}, 1)
	data, err := os.ReadFile(built)
	require.NoError(t, err)
	from := writeFile(t, filepath.Join(workDir, "old.AppImage"), data)

	err = runPortable(t, wizstep.Request{WorkDir: workDir}, "old.AppImage", ".", "")

	require.ErrorIs(t, err, unpack.ErrUnsupportedAppImage)
	assert.FileExists(t, from)
}

func TestHandler_Execute_AppImageFailures(t *testing.T) {
	testCases := []struct {
		name    string
		entries map[string]unpacktest.Entry
		to      string
		setup   func(t *testing.T, workDir string)
		wantMsg string
	}{
		{
			name:    "destination is a file",
			entries: appImageEntries("bruno"),
			to:      "blocker",
			setup: func(t *testing.T, workDir string) {
				writeFile(t, filepath.Join(workDir, "blocker"), []byte("x"))
			},
			wantMsg: "portable: remove leftover",
		},
		{
			name: "launcher location is a directory",
			entries: map[string]unpacktest.Entry{
				"AppRun":                           {Mode: 0o755, Data: "#!/bin/sh\n"},
				unpack.LauncherName + "/blocker":   {Mode: 0o644, Data: "x"},
				unpack.LauncherName + "/blocker-2": {Mode: 0o644, Data: "x"},
			},
			to:      ".",
			setup:   func(t *testing.T, workDir string) {},
			wantMsg: "unpack: launcher",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			writeAppImage(t, workDir, "bruno.AppImage", tc.entries)
			tc.setup(t, workDir)

			err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", tc.to, "")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
			assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
		})
	}
}

func TestHandler_Execute_AppImageUnreadableDesktopFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not block reads here")
	}

	const markerPerm = 0o604
	workDir := t.TempDir()
	built := unpacktest.BuildAppImage(t, map[string]unpacktest.Entry{
		"AppRun":        {Mode: 0o755, Data: "#!/bin/sh\n"},
		"bruno.desktop": {Mode: markerPerm, Data: "[Desktop Entry]\nName=Bruno\n"},
	}, 2)
	data, err := os.ReadFile(built)
	require.NoError(t, err)
	fileInode := binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint16(nil, 2), markerPerm)
	require.Equal(t, 1, bytes.Count(data, fileInode))
	unreadable := binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint16(nil, 2), 0o200)
	writeFile(t, filepath.Join(workDir, "bruno.AppImage"), bytes.Replace(data, fileInode, unreadable, 1))

	err = runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unpack: appimage metadata")
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

func TestHandler_Execute_AppImageReinstallRemovesStaleFiles(t *testing.T) {
	workDir := t.TempDir()
	req := wizstep.Request{WorkDir: workDir}
	old := appImageEntries("bruno")
	old["usr/lib/libold.so"] = unpacktest.Entry{Mode: 0o644, Data: "old"}
	writeAppImage(t, workDir, "bruno.AppImage", old)
	require.NoError(t, runPortable(t, req, "bruno.AppImage", ".", ""))
	require.FileExists(t, filepath.Join(workDir, "bruno", "usr", "lib", "libold.so"))

	next := appImageEntries("bruno")
	next["usr/lib/libnew.so"] = unpacktest.Entry{Mode: 0o644, Data: "new"}
	writeAppImage(t, workDir, "bruno.AppImage", next)
	require.NoError(t, runPortable(t, req, "bruno.AppImage", ".", ""))

	assert.NoFileExists(t, filepath.Join(workDir, "bruno", "usr", "lib", "libold.so"))
	assert.Equal(t, "new", unpacktest.ReadString(t, filepath.Join(workDir, "bruno", "usr", "lib", "libnew.so")))
	assert.FileExists(t, filepath.Join(workDir, "bruno", unpack.LauncherName))
}

func TestHandler_Execute_AppImageReadOnlyDestination(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not block writes here")
	}

	workDir := t.TempDir()
	writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))
	locked := filepath.Join(workDir, "apps")
	require.NoError(t, os.Mkdir(locked, 0o555))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", "apps", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unpack: create")
}

func TestHandler_Execute_AppImageCraftedNamesStayInsideDestination(t *testing.T) {
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
			parentSentinel := writeFile(t, filepath.Join(workDir, "keep"), []byte("x"))
			toSentinel := writeFile(t, filepath.Join(to, "keep"), []byte("x"))
			grandSentinel := writeFile(t, filepath.Join(parent, "keep"), []byte("x"))
			writeAppImage(t, workDir, tc.file, appImageEntries("bruno"))

			err := runPortable(t, wizstep.Request{WorkDir: workDir}, tc.file, "apps", "")

			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(to, tc.wantAppDir, unpack.LauncherName))
			assert.FileExists(t, parentSentinel)
			assert.FileExists(t, toSentinel)
			assert.FileExists(t, grandSentinel)
		})
	}
}

func TestHandler_Execute_AppImageRecordPointsAtFinalAppDir(t *testing.T) {
	workDir := t.TempDir()
	req := wizstep.Request{WorkDir: workDir}
	want := domain.PortableRecord{Apps: []domain.PortableApp{{
		Name:  "bruno",
		Entry: "apps/bruno/.quiver-run",
		Icon:  "apps/bruno/usr/share/icons/hicolor/256x256/apps/bruno.png",
	}}}

	for range 2 {
		writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))

		require.NoError(t, runPortable(t, req, "bruno.AppImage", "apps", ""))

		assert.Equal(t, want, readRecord(t, workDir))
		assert.FileExists(t, filepath.Join(workDir, "apps", "bruno", unpack.LauncherName))
		assert.NoDirExists(t, filepath.Join(workDir, "apps", ".bruno.quiver-tmp"))
	}
}

func TestHandler_Execute_AppImageCorruptKeepsExistingAppDir(t *testing.T) {
	workDir := t.TempDir()
	req := wizstep.Request{WorkDir: workDir}
	writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))
	require.NoError(t, runPortable(t, req, "bruno.AppImage", ".", ""))
	launcher := filepath.Join(workDir, "bruno", unpack.LauncherName)
	before := unpacktest.ReadString(t, launcher)

	built := unpacktest.BuildAppImage(t, appImageEntries("bruno"), 2)
	data, err := os.ReadFile(built)
	require.NoError(t, err)
	writeFile(t, filepath.Join(workDir, "bruno.AppImage"), data[:len(data)-len(data)/4])

	err = runPortable(t, req, "bruno.AppImage", ".", "")

	require.Error(t, err)
	assert.Equal(t, before, unpacktest.ReadString(t, launcher))
	assert.FileExists(t, filepath.Join(workDir, "bruno", "AppRun"))
	assert.NoDirExists(t, filepath.Join(workDir, ".bruno.quiver-tmp"))
}

func TestHandler_Execute_AppImageUnownedAppDirUntouched(t *testing.T) {
	workDir := t.TempDir()
	keep := writeFile(t, filepath.Join(workDir, "bruno", "notes.txt"), []byte("mine"))
	writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.ErrorIs(t, err, portable.ErrUnownedAppDir)
	assert.Contains(t, err.Error(), filepath.Join(workDir, "bruno"))
	assert.Equal(t, "mine", unpacktest.ReadString(t, keep))
	assert.NoFileExists(t, filepath.Join(workDir, "bruno", unpack.LauncherName))
	assert.NoDirExists(t, filepath.Join(workDir, ".bruno.quiver-tmp"))
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
}

func TestHandler_Execute_AppImageCleansLeftoverStaging(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, ".bruno.quiver-tmp", "usr", "lib", "crashed.so"), []byte("x"))
	writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(workDir, ".bruno.quiver-tmp"))
	assert.NoFileExists(t, filepath.Join(workDir, "bruno", "usr", "lib", "crashed.so"))
	assert.FileExists(t, filepath.Join(workDir, "bruno", unpack.LauncherName))
}
