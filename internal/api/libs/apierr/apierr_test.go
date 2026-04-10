package apierr_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/libs/apierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThrow_SetsFields(t *testing.T) {
	err := apierr.Throw(apierr.NotFoundCode, "not found", nil)

	assert.Equal(t, apierr.NotFoundCode, err.Code)
	assert.Equal(t, "not found", err.Message)
	assert.Nil(t, err.Details)
}

func TestError_FormatsCodeAndMessage(t *testing.T) {
	err := apierr.Throw(apierr.NotFoundCode, "resource missing", nil)

	assert.Equal(t, "404: resource missing", err.Error())
}

func TestThrow_WithDetails(t *testing.T) {
	details := map[string]interface{}{"field": "id"}
	err := apierr.Throw(apierr.UnprocessableEntityCode, "validation failed", details)

	assert.Equal(t, details, err.Details)
}

func TestConvenienceConstructors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code apierr.ErrorCode
	}{
		{"InvalidRequest", apierr.InvalidRequest("bad"), apierr.InvalidRequestCode},
		{"Unauthorized", apierr.Unauthorized("unauth"), apierr.UnauthorizedCode},
		{"Forbidden", apierr.Forbidden("forbidden"), apierr.ForbiddenCode},
		{"NotFound", apierr.NotFound("missing"), apierr.NotFoundCode},
		{"Conflict", apierr.Conflict("dup"), apierr.ConflictCode},
		{"InternalServerError", apierr.InternalServerError("boom"), apierr.InternalServerCode},
		{"HTTPVersionError", apierr.HTTPVersionError("old"), apierr.HTTPVersionErrorCode},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var apiErr apierr.Error
			require.ErrorAs(t, tc.err, &apiErr)
			assert.Equal(t, tc.code, apiErr.Code)
		})
	}
}
