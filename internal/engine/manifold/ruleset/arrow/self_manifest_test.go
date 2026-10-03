package arrow_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	translatorPkg "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
	arrowv0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow/v0"
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

func TestSelfManifest_UpdateDownloadsAndHandsOverPerPlatform(t *testing.T) {
	module, err := translatorPkg.NewTranslator().Arrow(selfmanifest.Raw())
	require.NoError(t, err)

	releases := "https://github.com/rabbytesoftware/quiver.core/releases/download/${REF}/"
	testCases := []struct {
		os    domain.OS
		asset string
		to    string
	}{
		{domain.OSDarwinARM64, "quiver-darwin-arm64", "${WORKDIR}/quiver-new"},
		{domain.OSDarwinAMD64, "quiver-darwin-amd64", "${WORKDIR}/quiver-new"},
		{domain.OSLinuxAMD64, "quiver-linux-amd64", "${WORKDIR}/quiver-new"},
		{domain.OSLinuxARM64, "quiver-linux-arm64", "${WORKDIR}/quiver-new"},
		{domain.OSWindowsAMD64, "quiver-windows-amd64.exe", `${WORKDIR}\quiver-new.exe`},
		{domain.OSWindowsARM64, "quiver-windows-amd64.exe", `${WORKDIR}\quiver-new.exe`},
	}

	for _, tc := range testCases {
		t.Run(tc.os.String(), func(t *testing.T) {
			target, err := arrowv0.SelectTarget(module.Precompiled, tc.os)
			require.NoError(t, err)
			require.Len(t, target.Lifecycle.Update, 2)
			fetch, ok := target.Lifecycle.Update[0].(domainStep.FetchStep)
			require.True(t, ok, "the first step downloads the new binary")
			run, ok := target.Lifecycle.Update[1].(domainStep.RunStep)
			require.True(t, ok, "the second step hands over to it")

			assert.Equal(t, releases+tc.asset, fetch.URL.Default)
			assert.Equal(t, tc.to, fetch.To.Default)
			assert.Equal(t, "sha256sums:"+releases+"checksums.txt", fetch.Checksum.Default)
			assert.Contains(t, run.Command.Default, `"`+tc.to+`" self-update "`+tc.to+`"`)
		})
	}
}
