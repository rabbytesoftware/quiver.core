package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestRefusal_JSON(t *testing.T) {
	data, err := json.Marshal(Refusal{Kind: domain.ExposeKindDesktop, Name: "App", Reason: ReasonUnmanaged})
	require.NoError(t, err)

	assert.JSONEq(t, `{"kind":"desktop","name":"App","reason":"exists and is not managed by quiver"}`, string(data))
}
