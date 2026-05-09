package assemblerinternal_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	assemblerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestResolveTarget_Found(t *testing.T) {
	os := domain.OSDarwinARM64
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			os: {Exports: map[string]string{"bin": "/usr/bin/foo"}},
		},
	}
	target, err := assemblerinternal.ResolveTarget(arrow, os)
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/foo", target.Exports["bin"])
}

func TestResolveTarget_NotFound(t *testing.T) {
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {},
		},
	}
	_, err := assemblerinternal.ResolveTarget(arrow, domain.OSDarwinARM64)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrPlatformNotSupported)
}

func TestResolveTarget_EmptyTargets(t *testing.T) {
	arrow := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{},
	}
	_, err := assemblerinternal.ResolveTarget(arrow, domain.OSDarwinARM64)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrPlatformNotSupported)
}
