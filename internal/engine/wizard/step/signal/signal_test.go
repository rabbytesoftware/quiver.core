package signal

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSignal_Universal(t *testing.T) {
	cases := []struct {
		name     string
		expected syscall.Signal
	}{
		{"SIGTERM", syscall.SIGTERM},
		{"SIGINT", syscall.SIGINT},
		{"SIGKILL", syscall.SIGKILL},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, err := ParseSignal(tc.name)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, sig)
		})
	}
}

func TestParseSignal_CaseInsensitive(t *testing.T) {
	variants := []string{"SIGTERM", "sigterm", "SigTerm", "sIgTeRm"}

	for _, v := range variants {
		t.Run(v, func(t *testing.T) {
			sig, err := ParseSignal(v)
			require.NoError(t, err)
			assert.Equal(t, syscall.SIGTERM, sig)
		})
	}
}

func TestParseSignal_Unknown(t *testing.T) {
	cases := []string{"SIGUNKNOWN", "NOTASIGNAL", "", "SIG", "9"}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			sig, err := ParseSignal(name)
			assert.Error(t, err)
			assert.Nil(t, sig)
		})
	}
}
