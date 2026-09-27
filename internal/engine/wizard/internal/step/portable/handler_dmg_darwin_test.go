//go:build darwin

package portable_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func buildDmg(
	t *testing.T,
	src string,
	out string,
) {
	t.Helper()

	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil not available")
	}

	output, err := exec.Command("hdiutil", "create", "-quiet", "-srcfolder", src, "-format", "UDZO", out).CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestHandler_Execute_DmgRecordsBundles(t *testing.T) {
	workDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "src")
	writeFile(t, filepath.Join(src, "Foo.app", "Contents", "MacOS", "foo"), []byte("bin"))
	writeFile(t, filepath.Join(src, "README.txt"), []byte("readme"))
	from := filepath.Join(workDir, "Foo.dmg")
	buildDmg(t, src, from)

	err := runPortable(t, wizstep.Request{WorkDir: workDir, OSArch: "darwin/arm64"}, "Foo.dmg", "apps", "5m")

	require.NoError(t, err)
	assert.Equal(t, "bin", unpacktest.ReadString(t, filepath.Join(workDir, "apps", "Foo.app", "Contents", "MacOS", "foo")))
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{{
		Name:  "Foo",
		Entry: "apps/Foo.app",
	}}}, readRecord(t, workDir))
	_, err = os.Stat(from)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestHandler_Execute_DmgDestinationIsAFile(t *testing.T) {
	workDir := t.TempDir()
	data := make([]byte, 1024)
	copy(data[512:], "koly")
	writeFile(t, filepath.Join(workDir, "Foo.dmg"), data)
	writeFile(t, filepath.Join(workDir, "apps"), []byte("x"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "Foo.dmg", "apps", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unpack: create")
}
