//go:build !windows

package shelf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedUserPath(t *testing.T) {
	_, err := defaultUserPath.read()
	assert.Error(t, err)
	assert.Error(t, defaultUserPath.write("x"))
}
