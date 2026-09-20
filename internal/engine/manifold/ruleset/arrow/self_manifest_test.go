package arrow_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	translatorPkg "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
)

func TestSelfManifest_ParsesAsValidArrowV0(t *testing.T) {
	trans := translatorPkg.NewTranslator()

	module, err := trans.Arrow(selfmanifest.Raw())
	require.NoError(t, err, "self-manifest must be valid according to arrow@v0 schema")

	wildcard, ok := module.Precompiled["*"]
	require.True(t, ok, "targets must define a wildcard OS entry")
	require.NotEmpty(t, wildcard.Lifecycle.Update, "self-manifest must define an update method")
	require.Empty(t, wildcard.Lifecycle.Install, "self-manifest must not define install — it is already installed by definition")
}

func TestSelfManifest_ValidatesAgainstSchema(t *testing.T) {
	trans := translatorPkg.NewTranslator()

	_, err := trans.Arrow(selfmanifest.Raw())
	require.NoError(t, err, "self-manifest must be valid according to arrow@v0 schema")
}

func TestSelfManifest_Namespace(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core")
	require.NoError(t, ns.Validate())
}
