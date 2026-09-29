//go:build darwin

package install_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
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

func TestInstall_DmgRecordsBundles(t *testing.T) {
	workDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "src")
	mocks.WriteFile(t, filepath.Join(src, "Foo.app", "Contents", "MacOS", "foo"), []byte("bin"))
	mocks.WriteFile(t, filepath.Join(src, "README.txt"), []byte("readme"))
	from := filepath.Join(workDir, "Foo.dmg")
	buildDmg(t, src, from)

	err := runPortable(t, wizstep.Request{WorkDir: workDir, OSArch: "darwin/arm64"}, "Foo.dmg", "apps", "5m")

	require.NoError(t, err)
	assert.Equal(t, "bin", mocks.ReadString(t, filepath.Join(workDir, "apps", "Foo.app", "Contents", "MacOS", "foo")))
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{{
		Name:  "Foo",
		Entry: "apps/Foo.app",
	}}}, readRecord(t, workDir))
	_, err = os.Stat(from)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestInstall_DmgDestinationIsAFile(t *testing.T) {
	workDir := t.TempDir()
	data := make([]byte, 1024)
	copy(data[512:], "koly")
	mocks.WriteFile(t, filepath.Join(workDir, "Foo.dmg"), data)
	mocks.WriteFile(t, filepath.Join(workDir, "apps"), []byte("x"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "Foo.dmg", "apps", "")

	require.Error(t, err)
	assert.Equal(t, "x", mocks.ReadString(t, filepath.Join(workDir, "apps")))
}
