package session

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
)

// activeContext's own cfg.Get lookup can only fail when it disagrees with
// cfg.Resolve's identical precedence, which Client never lets happen — so
// this defensive branch is exercised directly, bypassing Client.
func TestActiveContext_UnknownContext_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "cli.yaml"))
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "real", Server: "tcp://a:1"}, true))

	s := &session{flags: &Flags{Context: "ghost"}}
	name, token := s.activeContext(cfg)

	assert.Empty(t, name)
	assert.Empty(t, token)
}
