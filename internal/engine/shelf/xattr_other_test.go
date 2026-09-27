//go:build !darwin

package shelf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedTagger(t *testing.T) {
	_, err := defaultTagger.read(t.TempDir())
	assert.Error(t, err)
	assert.Error(t, defaultTagger.write(t.TempDir(), bareA.String()))
}
