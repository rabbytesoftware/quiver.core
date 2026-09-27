//go:build !darwin

package extract_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Execute_DmgUnsupported(t *testing.T) {
	dir := t.TempDir()
	image := writeArchive(t, dir, "app.dmg", []byte("image"))

	err := runExtract(t, testMaxBytes, image, filepath.Join(dir, "out"), "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract: dmg unsupported on")
}
