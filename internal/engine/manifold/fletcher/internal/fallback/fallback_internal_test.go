package fallback

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

func TestFetchFailure(t *testing.T) {
	transient := errors.New("repo page: HTTP 503")
	alreadyFailed := fmt.Errorf("readme: %w", resolver.ErrFetchFailed)
	notFletchable := models.NotFletchableError{Reason: models.ReasonNoUsableAsset}

	testCases := []struct {
		name       string
		err        error
		want       error
		wantAbsent error
		wantSame   bool
	}{
		{name: "transient error becomes a fetch failure", err: transient, want: resolver.ErrFetchFailed, wantAbsent: resolver.ErrNotFound},
		{name: "fetch failure is kept as is", err: alreadyFailed, want: resolver.ErrFetchFailed, wantAbsent: resolver.ErrNotFound, wantSame: true},
		{name: "not fletchable becomes a missing manifest", err: notFletchable, want: resolver.ErrManifestNotFound, wantAbsent: resolver.ErrFetchFailed},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := fetchFailure(tc.err)

			assert.ErrorIs(t, got, tc.err)
			assert.ErrorIs(t, got, tc.want)
			assert.NotErrorIs(t, got, tc.wantAbsent)
			if tc.wantSame {
				assert.Same(t, tc.err, got)
			}
		})
	}
}
