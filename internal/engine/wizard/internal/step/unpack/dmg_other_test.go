//go:build !darwin

package unpack_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func TestExtractDmg_Unsupported(t *testing.T) {
	dir := t.TempDir()
	image := unpacktest.WriteArchive(t, dir, "app.dmg", []byte("image"))
	to := filepath.Join(dir, "out")

	g, err := unpack.OpenGuard(context.Background(), to, unpacktest.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()

	err = unpack.ExtractDmg(context.Background(), image, g)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unpack: dmg unsupported on")
}
