package runtime

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

func TestStepProgress_JSONRoundTrip_WithStep(t *testing.T) {
	s := step.NewRunStep("do thing", "echo hi", false, "5s", true)
	original := StepProgress{
		Index:  0,
		Status: StepStatusRunning,
		Step:   s,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got StepProgress
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, original.Index, got.Index)
	assert.Equal(t, original.Status, got.Status)
	require.NotNil(t, got.Step, "Step is nil after round-trip")
	assert.Equal(t, step.StepTypeRun, got.Step.Type())
}

func TestStepProgress_JSONRoundTrip_WithError(t *testing.T) {
	errMsg := "something went wrong"
	original := StepProgress{
		Index:  2,
		Status: StepStatusFailed,
		Error:  &errMsg,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got StepProgress
	require.NoError(t, json.Unmarshal(data, &got))

	require.NotNil(t, got.Error, "Error is nil after round-trip")
	assert.Equal(t, errMsg, *got.Error)
	assert.Nil(t, got.Step)
}

func TestStepProgress_JSONRoundTrip_NilStep(t *testing.T) {
	original := StepProgress{
		Index:  1,
		Status: StepStatusPending,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got StepProgress
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Nil(t, got.Step)
	assert.Equal(t, 1, got.Index)
}

func TestStepProgress_UnmarshalJSON_EmptyStepArray(t *testing.T) {
	// Explicitly marshal JSON with Step: [] (empty array) to ensure
	// the len(raw.Step) == 0 branch is executed.
	raw := []byte(`{"Index":0,"Status":"pending","Error":null,"Step":[]}`)

	var got StepProgress
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Nil(t, got.Step)
	assert.Equal(t, 0, got.Index)
}

func TestStepProgress_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var sp StepProgress
	err := json.Unmarshal([]byte(`not valid json`), &sp)
	assert.Error(t, err)
}
