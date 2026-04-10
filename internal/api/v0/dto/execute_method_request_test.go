package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteMethodRequestDTO_MarshalUnmarshal(t *testing.T) {
	original := dto.ExecuteMethodRequestDTO{
		Variables: map[string]string{"KEY": "value", "PORT": "8080"},
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded dto.ExecuteMethodRequestDTO
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Variables, decoded.Variables)
}

func TestExecuteMethodRequestDTO_UnmarshalEmptyJSON(t *testing.T) {
	var d dto.ExecuteMethodRequestDTO
	require.NoError(t, json.Unmarshal([]byte(`{}`), &d))
	assert.Nil(t, d.Variables)
}
