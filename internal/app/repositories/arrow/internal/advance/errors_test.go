package advance_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/advance"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
)

func TestMapResolveErr_Classifies(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want error
	}{
		{name: "a ref that holds no manifest", err: fmt.Errorf("at v1.3.0: %w", manifoldresolver.ErrManifestNotFound), want: apperrors.ErrNotFound},
		{name: "a definitive not found", err: manifoldresolver.ErrNotFound, want: apperrors.ErrNotFound},
		{name: "an invalid manifest", err: ruleset.ErrInvalidManifest, want: apperrors.ErrInvalidManifest},
		{name: "an unsupported platform", err: ruleset.ErrNoSupportedPlatform, want: apperrors.ErrPlatformNotSupported},
		{name: "an already classified error", err: fmt.Errorf("x: %w", apperrors.ErrStateViolation), want: apperrors.ErrStateViolation},
		{name: "an unreachable remote", err: errors.New("dial tcp: refused"), want: apperrors.ErrFetchFailed},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, advance.MapResolveErr(tc.err), tc.want)
		})
	}
	assert.NoError(t, advance.MapResolveErr(nil))
}
