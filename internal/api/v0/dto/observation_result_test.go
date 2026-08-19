package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

func TestObservationResult_JSONRoundtrip(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	result := dto.ObservationResult{
		Items: []dto.ObservedItem{
			{
				Kind: "arrow",
				Arrow: &dto.ArrowStateDTO{
					Namespace: "github.com/user/repo",
					Name:      "repo",
					State:     "ready",
				},
			},
		},
		SnapshotTime: now,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var restored dto.ObservationResult
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, result.SnapshotTime, restored.SnapshotTime)
	assert.Equal(t, len(result.Items), len(restored.Items))
}

func TestObservationResult_YAMLRoundtrip(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	result := dto.ObservationResult{
		Items:        []dto.ObservedItem{},
		SnapshotTime: now,
	}

	data, err := yaml.Marshal(result)
	require.NoError(t, err)

	var restored dto.ObservationResult
	require.NoError(t, yaml.Unmarshal(data, &restored))

	assert.Equal(t, result.SnapshotTime, restored.SnapshotTime)
}

func TestObservationResult_CheckPayload_ValidResult(t *testing.T) {
	result := dto.ObservationResult{
		Items:        []dto.ObservedItem{},
		SnapshotTime: time.Now().UTC().Format(time.RFC3339),
	}

	assert.NoError(t, result.CheckPayload())
}

func TestObservationResult_CheckPayload_NilItems(t *testing.T) {
	result := dto.ObservationResult{
		Items:        nil,
		SnapshotTime: time.Now().UTC().Format(time.RFC3339),
	}

	assert.Error(t, result.CheckPayload())
}

func TestObservationResult_CheckPayload_InvalidTimestamp(t *testing.T) {
	result := dto.ObservationResult{
		Items:        []dto.ObservedItem{},
		SnapshotTime: "not-a-timestamp",
	}

	assert.Error(t, result.CheckPayload())
}
