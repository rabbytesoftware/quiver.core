//go:build !windows

package pathenv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedUserPath(t *testing.T) {
	userPath := NewUserPath()

	_, err := userPath.Read()
	assert.Error(t, err)
	assert.Error(t, userPath.Write("x"))
}
