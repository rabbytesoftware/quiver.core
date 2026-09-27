package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestInferenceDTOFrom_Nil(t *testing.T) {
	assert.Nil(t, dto.InferenceDTOFrom(nil))
}

func TestInferenceDTOFrom_MapsAllFields(t *testing.T) {
	got := dto.InferenceDTOFrom(&domain.ArrowGenerator{
		Name:       "fletcher",
		Confidence: "low",
		Warnings:   []string{"name_mismatch", "emulated"},
	})

	require.NotNil(t, got)
	assert.Equal(t, "fletcher", got.Generator)
	assert.Equal(t, "low", got.Confidence)
	assert.Equal(t, []string{"name_mismatch", "emulated"}, got.Warnings)
}
