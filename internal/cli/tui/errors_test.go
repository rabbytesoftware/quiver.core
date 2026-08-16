package tui_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

func TestCodeFor_MapsErrorTypesToExitCodes(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want int
	}{
		{"nil is success", nil, tui.ExitOK},
		{"plain error is failure", errors.New("boom"), tui.ExitFailure},
		{"usage error", tui.Usage("unknown output format %q", "xml"), tui.ExitUsage},
		{"connection error", tui.Conn("http://h:9500", errors.New("refused")), tui.ExitUnreachable},
		{"wrapped usage error", fmt.Errorf("outer: %w", tui.Usage("bad")), tui.ExitUsage},
		{"wrapped conn error", fmt.Errorf("outer: %w", tui.Conn("h", errors.New("x"))), tui.ExitUnreachable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tui.CodeFor(tc.err))
		})
	}
}

func TestUsage_Message(t *testing.T) {
	assert.Equal(t, `unknown output format "xml"`,
		tui.Usage("unknown output format %q", "xml").Error())
}

func TestConn_MessageAndUnwrap(t *testing.T) {
	inner := errors.New("connection refused")
	err := tui.Conn("http://host:9500", inner)

	assert.Equal(t, "cannot reach daemon at http://host:9500: connection refused", err.Error())
	assert.ErrorIs(t, err, inner)
}
