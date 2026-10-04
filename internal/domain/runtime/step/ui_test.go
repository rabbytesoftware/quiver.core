package step

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUIStep_RoundTrip(t *testing.T) {
	in := NewUIStep("Chat", []string{"unix"}, "", "/app", true)

	raw, err := json.Marshal(in)
	require.NoError(t, err)

	var out UIStep
	require.NoError(t, json.Unmarshal(raw, &out))

	require.Equal(t, StepTypeUI, out.Type())
	require.Equal(t, "Chat", out.Title())
	require.True(t, out.ExitOnFailure())
	require.Equal(t, []string{"unix"}, out.Listen)
	require.Equal(t, "/app", out.Path)
}

func TestUIStep_ResolveIsIdentity(t *testing.T) {
	s := NewUIStep("Docs", nil, "./dist", "", false)
	require.Equal(t, s, s.Resolve("linux/amd64"))
}

func TestStepFactory_KnowsUI(t *testing.T) {
	factory, ok := stepFactories[string(StepTypeUI)]
	require.True(t, ok)
	require.IsType(t, &UIStep{}, factory())
}

func TestDerefStep_UI(t *testing.T) {
	s := NewUIStep("x", nil, "./d", "", true)
	require.Equal(t, s, derefStep(&s))
}

func TestUIStep_UnmarshalJSON_Invalid(t *testing.T) {
	var s UIStep
	require.Error(t, json.Unmarshal([]byte(`{"listen":"x"}`), &s))
}
