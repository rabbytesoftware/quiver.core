package theme_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func newTestTheme(t *testing.T) theme.Theme {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	var buf bytes.Buffer

	return theme.New(lipgloss.NewRenderer(&buf))
}

func TestTheme_State_KnownStates(t *testing.T) {
	th := newTestTheme(t)

	testCases := []struct {
		name  string
		state domain.ArrowState
		want  string
	}{
		{"ready", domain.ArrowStateReady, "● ready"},
		{"running", domain.ArrowStateRunning, "▶ running"},
		{"absent", domain.ArrowStateAbsent, "○ absent"},
		{"installing", domain.ArrowStateInstalling, "⇣ installing"},
		{"outdated", domain.ArrowStateOutdated, "▲ outdated"},
		{"stopping", domain.ArrowStateStopping, "◼ stopping"},
		{"draining", domain.ArrowStateDraining, "◍ draining"},
		{"detached", domain.ArrowStateDetached, "◈ detached"},
		{"uninstalling", domain.ArrowStateUninstalling, "⇡ uninstalling"},
		{"updating", domain.ArrowStateUpdating, "⇅ updating"},
		{"removed", domain.ArrowStateRemoved, "✕ removed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, th.State(tc.state))
		})
	}
}

func TestTheme_State_UnknownStateFallsBack(t *testing.T) {
	th := newTestTheme(t)

	assert.Equal(t, "? nonsense", th.State(domain.ArrowState("nonsense")))
}

func TestTheme_New_StylesAreUsable(t *testing.T) {
	th := newTestTheme(t)

	require.Equal(t, "hi", th.Muted.Render("hi"))
	require.Equal(t, "hi", th.Header.Render("hi"))
	require.Equal(t, "hi", th.Label.Render("hi"))
	require.Equal(t, "hi", th.Value.Render("hi"))
	require.Equal(t, "hi", th.OK.Render("hi"))
	require.Equal(t, "hi", th.Warn.Render("hi"))
	require.Equal(t, "hi", th.Fail.Render("hi"))
	require.Equal(t, "hi", th.Active.Render("hi"))
}
