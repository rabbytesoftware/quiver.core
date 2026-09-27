package portable_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

func TestHandler_Execute_RerunReplacesRecordEntry(t *testing.T) {
	workDir := t.TempDir()
	req := wizstep.Request{WorkDir: workDir}

	for range 2 {
		writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))
		require.NoError(t, runPortable(t, req, "bruno.AppImage", ".", ""))
	}
	writeAppImage(t, workDir, "zed.AppImage", appImageEntries("zed"))
	require.NoError(t, runPortable(t, req, "zed.AppImage", ".", ""))

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{
		{Name: "bruno", Entry: "bruno/.quiver-run", Icon: "bruno/usr/share/icons/hicolor/256x256/apps/bruno.png"},
		{Name: "zed", Entry: "zed/.quiver-run", Icon: "zed/usr/share/icons/hicolor/256x256/apps/zed.png"},
	}}, readRecord(t, workDir))
}

func TestHandler_Execute_MalformedRecordReplaced(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, domain.PortableRecordFile), []byte("{not json"))
	writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.NoError(t, err)
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{
		{Name: "bruno", Entry: "bruno/.quiver-run", Icon: "bruno/usr/share/icons/hicolor/256x256/apps/bruno.png"},
	}}, readRecord(t, workDir))
}
