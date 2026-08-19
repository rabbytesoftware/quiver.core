package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

func TestLifecycleOutcome_JSONRoundtrip(t *testing.T) {
	outcome := dto.LifecycleOutcome{
		Subject:    "github.com/user/repo",
		Action:     "install",
		Success:    true,
		FinalState: "ready",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Steps: []dto.StepRecord{
			{
				Name:     "fetch manifest",
				State:    "done",
				Duration: "0.3s",
			},
		},
	}

	data, err := json.Marshal(outcome)
	require.NoError(t, err)

	var restored dto.LifecycleOutcome
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, outcome.Subject, restored.Subject)
	assert.Equal(t, outcome.Success, restored.Success)
	assert.Equal(t, len(outcome.Steps), len(restored.Steps))
}

func TestLifecycleOutcome_CheckPayload_ValidOutcome(t *testing.T) {
	outcome := dto.LifecycleOutcome{
		Subject:    "github.com/user/repo",
		Action:     "install",
		Success:    true,
		FinalState: "ready",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Steps:      []dto.StepRecord{},
	}

	assert.NoError(t, outcome.CheckPayload())
}

func TestLifecycleOutcome_CheckPayload_InvalidAction(t *testing.T) {
	outcome := dto.LifecycleOutcome{
		Subject:    "github.com/user/repo",
		Action:     "invalid-action",
		Success:    true,
		FinalState: "ready",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Steps:      []dto.StepRecord{},
	}

	assert.Error(t, outcome.CheckPayload())
}

func TestLifecycleOutcome_CheckPayload_NilSteps(t *testing.T) {
	outcome := dto.LifecycleOutcome{
		Subject:    "github.com/user/repo",
		Action:     "install",
		Success:    true,
		FinalState: "ready",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Steps:      nil,
	}

	assert.Error(t, outcome.CheckPayload())
}
