package commands

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSpinnerTestCmd() (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	cmd.SetOut(&bytes.Buffer{})
	return cmd, &errBuf
}

func TestWithSpinner_NonTTYRunsSilently(t *testing.T) {
	a := &app{deps: Deps{IsTTY: func() bool { return false }}}
	cmd, errBuf := newSpinnerTestCmd()

	ran := false
	err := a.withSpinner(cmd, "loading", func() error { ran = true; return nil })

	require.NoError(t, err)
	assert.True(t, ran, "fn must run")
	assert.Empty(t, errBuf.String(), "no spinner output when not a TTY")
}

func TestWithSpinner_PropagatesError(t *testing.T) {
	a := &app{deps: Deps{IsTTY: func() bool { return false }}}
	cmd, _ := newSpinnerTestCmd()

	sentinel := errors.New("boom")
	err := a.withSpinner(cmd, "loading", func() error { return sentinel })

	assert.ErrorIs(t, err, sentinel)
}

func TestWithSpinner_FastTTYOpDoesNotFlash(t *testing.T) {
	a := &app{deps: Deps{IsTTY: func() bool { return true }}}
	cmd, errBuf := newSpinnerTestCmd()

	// fn returns immediately, well under spinnerDelay → nothing is drawn.
	err := a.withSpinner(cmd, "loading", func() error { return nil })

	require.NoError(t, err)
	assert.Empty(t, errBuf.String(), "a fast op must not flash the spinner")
}
