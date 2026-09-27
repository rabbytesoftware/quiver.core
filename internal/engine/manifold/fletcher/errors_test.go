package fletcher_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
)

func TestNotFletchableError(t *testing.T) {
	testCases := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "host unsupported", reason: fletcher.ReasonHostUnsupported, want: "fletcher: not fletchable: host_unsupported"},
		{name: "no release assets", reason: fletcher.ReasonNoReleaseAssets, want: "fletcher: not fletchable: no_release_assets"},
		{name: "no usable asset", reason: fletcher.ReasonNoUsableAsset, want: "fletcher: not fletchable: no_usable_asset"},
		{name: "disabled", reason: fletcher.ReasonDisabled, want: "fletcher: not fletchable: disabled"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", fletcher.NotFletchableError{Reason: tc.reason})

			var nf fletcher.NotFletchableError
			assert.True(t, errors.As(err, &nf))
			assert.Equal(t, tc.reason, nf.Reason)
			assert.ErrorIs(t, err, fletcher.ErrNotFletchable)
			assert.Equal(t, tc.want, nf.Error())
		})
	}
}
