package selfmanifest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
)

// TestSelfmanifest_Raw_ParsesAsRealArrowManifest confirms the embedded
// ARROW.md resolves at build time and parses through the exact same
// translator path a fetched ARROW.md would go through: Arrow() (which
// strips the ```arrow fence and validates the manifest against the arrow@v0
// schema) and ExtractReadme() (which pulls the surrounding prose). Arrow()
// alone never populates Manifest.Readme — that composition only happens one
// layer up, in manifold.ParseArrow — so both calls are exercised here to
// prove the embedded bytes work end to end on the translator's public API.
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
