//go:build !darwin

package ownership

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedTagger(t *testing.T) {
	tagger := NewTagger()

	_, err := tagger.Read(t.TempDir())
	assert.Error(t, err)
	assert.Error(t, tagger.Write(t.TempDir(), "x"))
}
