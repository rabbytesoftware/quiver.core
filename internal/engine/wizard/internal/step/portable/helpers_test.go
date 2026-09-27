package portable_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

const elfExecutable = "\x7fELF\x02\x01\x01\x00binary"

func runPortable(
	t *testing.T,
	req wizstep.Request,
	from string,
	to string,
	timeout string,
) error {
	t.Helper()

	h := portable.NewHandler(unpacktest.TestMaxBytes)
	s := domainstep.NewPortableStep("portable", from, to, timeout, true)

	return h.Execute(context.Background(), req, s)
}

func appImageEntries(
	name string,
) map[string]unpacktest.Entry {
	return map[string]unpacktest.Entry{
		"AppRun":          {Mode: 0o755, Data: "#!/bin/sh\n"},
		name + ".desktop": {Mode: 0o644, Data: "[Desktop Entry]\nName=" + name + "\nExec=AppRun --no-sandbox %U\nIcon=" + name + "\n"},
		"usr/share/icons/hicolor/256x256/apps/" + name + ".png": {Mode: 0o644, Data: "png"},
		".DirIcon": {Link: "/usr/share/pixmaps/" + name + ".png"},
	}
}

func writeAppImage(
	t *testing.T,
	dir string,
	file string,
	entries map[string]unpacktest.Entry,
) string {
	t.Helper()

	built := unpacktest.BuildAppImage(t, entries, 2)
	data, err := os.ReadFile(built)
	require.NoError(t, err)

	return writeFile(t, filepath.Join(dir, file), data)
}

func writeFile(
	t *testing.T,
	path string,
	data []byte,
) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o600))

	return path
}

func zipBytes(
	t *testing.T,
	entries map[string]string,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

func readRecord(
	t *testing.T,
	workDir string,
) domain.PortableRecord {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(workDir, domain.PortableRecordFile))
	require.NoError(t, err)

	var record domain.PortableRecord
	require.NoError(t, json.Unmarshal(data, &record))

	return record
}
