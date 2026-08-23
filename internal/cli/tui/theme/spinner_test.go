package theme_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func TestSpinner_Frame_HiddenUntilDelayElapsed(t *testing.T) {
	s := theme.NewSpinner()
	assert.Equal(t, "", s.Frame(), "spinner must not flash on fast commands")

	s, _ = s.Update(theme.TickMsg(time.Now()))
	assert.Equal(t, "", s.Frame())

	s, _ = s.Update(theme.TickMsg(time.Now()))
	assert.NotEqual(t, "", s.Frame(), "spinner must appear after the start delay")
}

func TestSpinner_Update_AdvancesFrameAndRearms(t *testing.T) {
	s := theme.NewSpinner()
	for range 2 {
		s, _ = s.Update(theme.TickMsg(time.Now()))
	}

	first := s.Frame()
	s, cmd := s.Update(theme.TickMsg(time.Now()))

	assert.NotEqual(t, first, s.Frame(), "frame must advance")
	require.NotNil(t, cmd, "tick must re-arm itself")
}

func TestSpinner_Update_IgnoresOtherMessages(t *testing.T) {
	s := theme.NewSpinner()
	for range 2 {
		s, _ = s.Update(theme.TickMsg(time.Now()))
	}

	before := s.Frame()
	s, cmd := s.Update("not a tick")

	assert.Equal(t, before, s.Frame())
	assert.Nil(t, cmd)
}

func TestSpinner_Update_FramesCycle(t *testing.T) {
	s := theme.NewSpinner()

	seen := map[string]bool{}
	for range 16 {
		s, _ = s.Update(theme.TickMsg(time.Now()))
		if f := s.Frame(); f != "" {
			seen[f] = true
		}
	}

	assert.Len(t, seen, 8, "the full frame set must cycle")
}

func TestSpinner_Tick_ReturnsTickMsg(t *testing.T) {
	msg := theme.NewSpinner().Tick()()

	assert.IsType(t, theme.TickMsg{}, msg)
}
