//go:build !darwin

package install_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestInstall_DmgUnsupportedOffDarwin(t *testing.T) {
	workDir := t.TempDir()
	data := make([]byte, 1024)
	copy(data[512:], "koly")
	from := mocks.WriteFile(t, filepath.Join(workDir, "Foo.dmg"), data)

	err := runPortable(t, wizstep.Request{WorkDir: workDir, OSArch: "darwin/arm64"}, "Foo.dmg", ".", "")

	require.Error(t, err)
	assert.FileExists(t, from)
}
