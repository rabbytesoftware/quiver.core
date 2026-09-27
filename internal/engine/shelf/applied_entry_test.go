package shelf

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestAppliedEntry_JSON(t *testing.T) {
	data, err := json.Marshal(AppliedEntry{Kind: domain.ExposeKindCLI, Name: "rg", Target: "/t", Location: "/l"})
	require.NoError(t, err)

	assert.JSONEq(t, `{"kind":"cli","name":"rg","target":"/t","location":"/l"}`, string(data))
}
