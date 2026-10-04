//go:build !windows

package selfupdate

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLaunch_FallsBackToTheNextAttemptWhenOneIsRefused(t *testing.T) {
	t.Setenv(helperEnvInternal, "exit")
	refused := &syscall.SysProcAttr{Chroot: "/nonexistent-dir-for-test"}

	pid, err := launch(os.Args[0], []string{"-test.run=^$"}, []*syscall.SysProcAttr{refused, {Setsid: true}})

	require.NoError(t, err)
	assert.Positive(t, pid)
}

func TestLaunch_EveryAttemptRefusedReportsThem(t *testing.T) {
	refused := &syscall.SysProcAttr{Chroot: "/nonexistent-dir-for-test"}

	_, err := launch(os.Args[0], nil, []*syscall.SysProcAttr{refused, refused})

	require.Error(t, err)
}

func TestDetachAttempts_UnixHasOne(t *testing.T) {
	assert.Len(t, detachAttempts(), 1)
}

const helperEnvInternal = "QUIVER_SELFUPDATE_INTERNAL_HELPER"
