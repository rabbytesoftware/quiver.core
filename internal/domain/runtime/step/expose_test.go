package step

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExposeStep(t *testing.T) {
	s := NewExposeStep("cli", "bat", "${INSTALL_PATH}/bat")

	assert.Equal(t, StepTypeExpose, s.Type())
	assert.Equal(t, "Expose cli bat", s.Title())
	assert.False(t, s.ExitOnFailure())
	assert.Equal(t, s, s.Resolve("linux/amd64"))
}

func TestNewUnexposeStep(t *testing.T) {
	s := NewUnexposeStep()

	assert.Equal(t, StepTypeUnexpose, s.Type())
	assert.Equal(t, "Remove exposed entries", s.Title())
	assert.False(t, s.ExitOnFailure())
	assert.Equal(t, s, s.Resolve("linux/amd64"))
}

func TestExposeSteps_StepListJSONRoundTrip(t *testing.T) {
	expose := NewExposeStep("desktop", "Tool", "auto")
	expose.Icon = "${INSTALL_PATH}/icon.png"
	expose.Categories = []string{"Development"}
	expose.MediaIcon = "/media.png"
	original := StepList{expose, NewUnexposeStep()}

	data, err := json.Marshal(original)
	require.NoError(t, err)
	var got StepList
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, original, got)
}

func TestExposeSteps_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var expose ExposeStep
	var unexpose UnexposeStep

	require.Error(t, expose.UnmarshalJSON([]byte("{")))
	require.Error(t, unexpose.UnmarshalJSON([]byte("{")))
}
