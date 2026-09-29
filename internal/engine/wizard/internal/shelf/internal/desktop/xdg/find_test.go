package xdg

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func TestExposer_Find_ResolvesAnAppImage(t *testing.T) {
	f := newFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "Tool-x86_64.AppImage"), "x", 0o755)

	got, reason, err := f.exposer.Find(req, domain.ExposeEntry{Name: "Tool", Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []models.Candidate{{Name: "Tool", Target: filepath.Join(wd, "Tool-x86_64.AppImage"), Depth: 1}}, got)
}
