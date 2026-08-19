package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

func TestInfoResult_JSONRoundtrip(t *testing.T) {
	result := dto.InfoResult{
		Kind: "arrow",
		Subject: map[string]interface{}{
			"namespace": "github.com/user/repo",
			"name":      "repo",
		},
		RelatedInfo: map[string]any{
			"methods": []string{"health", "logs"},
		},
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var restored dto.InfoResult
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, result.Kind, restored.Kind)
	assert.NotNil(t, restored.Subject)
}

func TestInfoResult_CheckPayload_ValidResult(t *testing.T) {
	result := dto.InfoResult{
		Kind:        "arrow",
		Subject:     map[string]interface{}{},
		RelatedInfo: map[string]any{},
	}

	assert.NoError(t, result.CheckPayload())
}

func TestInfoResult_CheckPayload_InvalidKind(t *testing.T) {
	result := dto.InfoResult{
		Kind:        "invalid",
		Subject:     map[string]interface{}{},
		RelatedInfo: map[string]any{},
	}

	assert.Error(t, result.CheckPayload())
}

func TestInfoResult_CheckPayload_NilSubject(t *testing.T) {
	result := dto.InfoResult{
		Kind:        "arrow",
		Subject:     nil,
		RelatedInfo: map[string]any{},
	}

	assert.Error(t, result.CheckPayload())
}
