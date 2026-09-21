package selfmanifest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
)

// Arrow() alone never populates Manifest.Readme — that composition happens
// one layer up, in manifold.ParseArrow — so ExtractReadme is called
// separately here to cover both.
func TestSelfmanifest_Raw_ParsesAsRealArrowManifest(t *testing.T) {
	raw := selfmanifest.Raw()
	require.NotEmpty(t, raw)

	tr := translator.NewTranslator()

	module, err := tr.Arrow(raw)
	require.NoError(t, err)
	require.Equal(t, "Quiver Core", module.Manifest.Name)

	readme, ok := tr.ExtractReadme(raw)
	require.True(t, ok)
	require.NotEmpty(t, readme)
}
