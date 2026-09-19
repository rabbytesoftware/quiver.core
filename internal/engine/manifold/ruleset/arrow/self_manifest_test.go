package arrow_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	translatorPkg "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
)

func TestSelfManifest_ParsesAsValidArrowV0(t *testing.T) {
	data, err := os.ReadFile("../../../../../arrow.yaml")
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, yaml.Unmarshal(data, &raw))
	require.Equal(t, "arrow@v0", raw["schema"])

	targets, ok := raw["targets"].(map[string]any)
	require.True(t, ok, "targets must be a map")
	wildcard, ok := targets["*"].(map[string]any)
	require.True(t, ok, "targets must define a wildcard OS entry")
	lifecycle, ok := wildcard["lifecycle"].(map[string]any)
	require.True(t, ok)
	_, hasUpdate := lifecycle["update"]
	require.True(t, hasUpdate, "self-manifest must define an update method")
	_, hasInstall := lifecycle["install"]
	require.False(t, hasInstall, "self-manifest must not define install — it is already installed by definition")
}

func TestSelfManifest_ValidatesAgainstSchema(t *testing.T) {
	data, err := os.ReadFile("../../../../../arrow.yaml")
	require.NoError(t, err)

	trans := translatorPkg.NewTranslator()
	_, err = trans.Arrow(data)
	require.NoError(t, err, "self-manifest must be valid according to arrow@v0 schema")
}

func TestSelfManifest_Namespace(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core")
	require.NoError(t, ns.Validate())
}
