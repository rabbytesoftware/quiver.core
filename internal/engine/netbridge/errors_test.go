package netbridge

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors_CanWrap(
	t *testing.T,
) {
	cases := []struct {
		name    string
		sentinel error
	}{
		{"ErrNoPortAvailable", ErrNoPortAvailable},
		{"ErrPortOutOfRange", ErrPortOutOfRange},
		{"ErrBuildIncomplete", ErrBuildIncomplete},
	}

	for _, tc := range cases {
		wrapped := fmt.Errorf("context: %w", tc.sentinel)
		assert.True(t, errors.Is(wrapped, tc.sentinel), "%s should be unwrappable", tc.name)
	}
}

func TestErrors_NotEqual(
	t *testing.T,
) {
	assert.NotEqual(t, ErrNoPortAvailable, ErrPortOutOfRange)
	assert.NotEqual(t, ErrPortOutOfRange, ErrBuildIncomplete)
	assert.NotEqual(t, ErrNoPortAvailable, ErrBuildIncomplete)
}
