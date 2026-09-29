package models_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
)

func TestNotFletchableError(t *testing.T) {
	testCases := []struct {
		name   string
		reason models.Reason
	}{
		{name: "host unsupported", reason: models.ReasonHostUnsupported},
		{name: "no release assets", reason: models.ReasonNoReleaseAssets},
		{name: "no usable asset", reason: models.ReasonNoUsableAsset},
		{name: "no digest", reason: models.ReasonNoDigest},
		{name: "low confidence", reason: models.ReasonLowConfidence},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", models.NotFletchableError{Reason: tc.reason})

			var nf models.NotFletchableError
			assert.True(t, errors.As(err, &nf))
			assert.Equal(t, tc.reason, nf.Reason)
			assert.ErrorIs(t, err, models.ErrNotFletchable)
		})
	}
}

func TestNotFletchableError_EveryReasonReadsDifferently(t *testing.T) {
	seen := make(map[string]models.Reason)
	for _, reason := range []models.Reason{
		models.ReasonHostUnsupported,
		models.ReasonNoReleaseAssets,
		models.ReasonNoUsableAsset,
		models.ReasonNoDigest,
		models.ReasonLowConfidence,
	} {
		message := models.NotFletchableError{Reason: reason}.Error()
		other, clash := seen[message]
		assert.False(t, clash, "%s and %s read the same", reason, other)
		seen[message] = reason
	}
}
