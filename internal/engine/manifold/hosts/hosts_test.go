package hosts_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

// stubHost is the shape the provider engine implements. It exists here only so
// a lookup has something to return.
type stubHost struct{}

func (stubHost) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "https://example.test/file", nil
}

func (stubHost) DefaultBranches() []string { return []string{"main"} }

func (stubHost) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "v1.0.0", nil
}

func TestNone_KnowsNoHost(t *testing.T) {
	host, ok := hosts.None(domain.Namespace("github.com/u/r"))
	assert.False(t, ok)
	assert.Nil(t, host)
}

// An unwired lookup is a manifold that knows no hosts, which resolves every
// namespace by cloning. It must never be a nil call.
func TestOr_NilFallsBackToNone(t *testing.T) {
	host, ok := hosts.Or(nil)(domain.Namespace("github.com/u/r"))
	assert.False(t, ok)
	assert.Nil(t, host)
}

func TestOr_KeepsTheLookupItWasGiven(t *testing.T) {
	lookup := func(_ domain.Namespace) (hosts.Host, bool) { return stubHost{}, true }

	host, ok := hosts.Or(lookup)(domain.Namespace("github.com/u/r"))
	require.True(t, ok)
	assert.Equal(t, []string{"main"}, host.DefaultBranches())
}
