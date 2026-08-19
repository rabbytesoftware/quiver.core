package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

func TestMutationResult_JSONRoundtrip(t *testing.T) {
	result := dto.MutationResult{
		Action:    "add",
		Subject:   "github.com/user/repo",
		Success:   true,
		Message:   "registered arrow and resolved manifest",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var restored dto.MutationResult
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, result.Action, restored.Action)
	assert.Equal(t, result.Subject, restored.Subject)
}

func TestMutationResult_CheckPayload_ValidResult(t *testing.T) {
	result := dto.MutationResult{
		Action:    "add",
		Subject:   "github.com/user/repo",
		Success:   true,
		Message:   "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	assert.NoError(t, result.CheckPayload())
}

func TestMutationResult_CheckPayload_InvalidAction(t *testing.T) {
	result := dto.MutationResult{
		Action:    "invalid",
		Subject:   "github.com/user/repo",
		Success:   true,
		Message:   "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	assert.Error(t, result.CheckPayload())
}
