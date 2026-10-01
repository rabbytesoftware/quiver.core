//go:build windows

package tui

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReaderGone_WindowsBrokenPipes(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "broken pipe", err: &os.PathError{Op: "write", Err: syscall.ERROR_BROKEN_PIPE}, want: true},
		{name: "pipe being closed", err: &os.PathError{Op: "write", Err: errNoData}, want: true},
		{name: "access denied", err: &os.PathError{Op: "write", Err: syscall.ERROR_ACCESS_DENIED}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, readerGone(tc.err))
		})
	}
}
