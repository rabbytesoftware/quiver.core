package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWizard(t *testing.T) {
	wizard := NewWizard()
	require.NotNil(t, wizard)
	assert.IsType(t, &Wizard{}, wizard)
}
