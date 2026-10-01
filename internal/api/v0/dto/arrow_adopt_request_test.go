package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

func TestAdoptRequestDTO_Unmarshal(t *testing.T) {
	testCases := []struct {
		name string
		body string
		want dto.AdoptRequestDTO
	}{
		{name: "resolved ref", body: `{"resolved_ref":"v1.2.0"}`, want: dto.AdoptRequestDTO{ResolvedRef: "v1.2.0"}},
		{name: "unknown fields are ignored", body: `{"resolved_ref":"v1.2.0","channel":"beta"}`, want: dto.AdoptRequestDTO{ResolvedRef: "v1.2.0"}},
		{name: "empty object", body: `{}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var got dto.AdoptRequestDTO
			require.NoError(t, json.Unmarshal([]byte(tc.body), &got))
			assert.Equal(t, tc.want, got)
		})
	}
}
