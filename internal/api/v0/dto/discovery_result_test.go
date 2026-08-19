package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

func TestDiscoveryResult_JSONRoundtrip(t *testing.T) {
	result := dto.DiscoveryResult{
		Items: []dto.DiscoveryItem{
			{
				Kind: "arrow",
				Arrow: &dto.ArrowListItemDTO{
					Namespace: "github.com/user/repo",
					Name:      "repo",
				},
			},
		},
		Total: 1,
		Query: "pattern",
	}

	// Marshal to JSON
	data, err := json.Marshal(result)
	require.NoError(t, err)

	// Unmarshal back
	var restored dto.DiscoveryResult
	require.NoError(t, json.Unmarshal(data, &restored))

	// Verify round-trip
	assert.Equal(t, result.Items[0].Kind, restored.Items[0].Kind)
	assert.Equal(t, result.Total, restored.Total)
	assert.Equal(t, result.Query, restored.Query)
}

func TestDiscoveryResult_YAMLRoundtrip(t *testing.T) {
	result := dto.DiscoveryResult{
		Items: []dto.DiscoveryItem{},
		Total: 0,
		Query: "",
	}

	data, err := yaml.Marshal(result)
	require.NoError(t, err)

	var restored dto.DiscoveryResult
	require.NoError(t, yaml.Unmarshal(data, &restored))

	assert.Equal(t, result.Total, restored.Total)
}

func TestDiscoveryResult_CheckPayload_ValidResult(t *testing.T) {
	result := dto.DiscoveryResult{
		Items: []dto.DiscoveryItem{},
		Total: 0,
		Query: "",
	}

	assert.NoError(t, result.CheckPayload())
}

func TestDiscoveryResult_CheckPayload_NilItems(t *testing.T) {
	result := dto.DiscoveryResult{
		Items: nil, // Invalid
		Total: 1,
	}

	assert.Error(t, result.CheckPayload())
}
