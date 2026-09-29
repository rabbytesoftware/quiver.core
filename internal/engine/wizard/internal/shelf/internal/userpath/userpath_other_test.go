//go:build !windows

package userpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupported(t *testing.T) {
	up := New()

	_, err := up.Read()
	assert.Error(t, err)
	assert.Error(t, up.Write("x"))
	assert.Error(t, up.Broadcast())
	assert.Equal(t, `HKCU\Environment\Path`, up.Location())
}
