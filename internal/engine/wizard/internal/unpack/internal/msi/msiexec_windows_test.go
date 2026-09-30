//go:build windows

package msi

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

// No MSI fixture can be built reliably on a stock runner, so this drives the
// real msiexec with a package that does not exist: it must run, exit with a
// failure code, and surface as ErrMsiexecFailed rather than hang or succeed.
func TestMsiexec_MissingPackageFails(t *testing.T) {
	work := t.TempDir()

	err := adminInstall(
		context.Background(),
		msiexec,
		filepath.Join(work, "missing package.msi"),
		filepath.Join(work, "image"),
		filepath.Join(work, "msiexec.log"),
	)

	require.ErrorIs(t, err, models.ErrMsiexecFailed)
}
