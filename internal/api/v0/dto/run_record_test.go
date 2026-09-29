package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestRunRecordDTO_JSON_EmptyStepsSerializeAsArray(t *testing.T) {
	raw, err := json.Marshal(dto.RunRecordDTOFrom(&domainRuntime.Execution{Method: domain.MethodUninstall}))
	require.NoError(t, err)

	assert.Contains(t, string(raw), `"steps":[]`)
}
