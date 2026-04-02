package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	w, err := New()
	require.NoError(t, err)
	require.NotNil(t, w)
	assert.Implements(t, (*Wizard)(nil), w)
}
