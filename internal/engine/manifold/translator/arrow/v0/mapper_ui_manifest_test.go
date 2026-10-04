package v0_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	v0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow/v0"
)

const uiManifestHead = `
schema: "arrow@v0"
metadata:
  name: ui-test
targets:
  "*":
    lifecycle:
      execute:
        - type: ui
`

func TestMap_UIStep_ListenCompilesToUIStep(t *testing.T) {
	data := []byte(uiManifestHead + `          title: Chat
          listen: [unix]
          path: /
`)
	require.NoError(t, validateAgainstSchema(t, v0.New().Schema(), data))

	_, precompiled, err := v0.New().Parse(data)
	require.NoError(t, err)

	execute := precompiled["*"].Lifecycle.Execute
	require.Len(t, execute, 1)
	got, ok := execute[0].(step.UIStep)
	require.True(t, ok, "execute[0] is %T, want UIStep", execute[0])
	require.Equal(t, step.NewUIStep("Chat", []string{"unix"}, "", "/", true), got)
}

func TestMap_UIStep_UpstreamRejectedBySchema(t *testing.T) {
	data := []byte(uiManifestHead + `          listen: [unix]
          upstream: x
`)
	require.Error(t, validateAgainstSchema(t, v0.New().Schema(), data))
}

func TestMap_UIStep_UnknownListenKindRejectedBySchema(t *testing.T) {
	data := []byte(uiManifestHead + `          listen: [tcp]
`)
	require.Error(t, validateAgainstSchema(t, v0.New().Schema(), data))
}
