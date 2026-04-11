package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommand_HasDaemonSubcommand(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"daemon"})
	require.NoError(t, err)
	assert.Equal(t, "daemon", cmd.Name())
}

func TestDaemonCommand_HasHostFlag(t *testing.T) {
	cmd := newDaemonCmd()
	assert.NotNil(t, cmd.Flags().Lookup("host"))
}

func TestDaemonCommand_HasPortFlag(t *testing.T) {
	cmd := newDaemonCmd()
	assert.NotNil(t, cmd.Flags().Lookup("port"))
}
