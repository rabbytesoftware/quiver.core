//go:build !windows

package signal

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSignal_Unix(t *testing.T) {
	cases := []struct {
		name     string
		expected syscall.Signal
	}{
		{"SIGHUP", syscall.SIGHUP},
		{"SIGUSR1", syscall.SIGUSR1},
		{"SIGUSR2", syscall.SIGUSR2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, err := ParseSignal(tc.name)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, sig)
		})
	}
}
