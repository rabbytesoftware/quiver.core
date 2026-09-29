package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestBundleApps_KeepsTopLevelAppDirectories(t *testing.T) {
	to := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(to, "Foo.app", "Contents"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(to, "Bar.app"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(to, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(to, "notes.app"), []byte("x"), 0o600))

	apps := bundleApps(to, []string{"Bar.app", "Foo.app", "docs", "missing.app", "notes.app"})

	assert.Equal(t, []domain.PortableApp{
		{Name: "Bar", Entry: filepath.Join(to, "Bar.app")},
		{Name: "Foo", Entry: filepath.Join(to, "Foo.app")},
	}, apps)
}

func TestBundleApps_NoNames(t *testing.T) {
	assert.Empty(t, bundleApps(t.TempDir(), nil))
}
