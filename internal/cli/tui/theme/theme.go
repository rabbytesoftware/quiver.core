// Package theme holds the CLI's visual vocabulary: colours, glyphs and the
// spinner, all bound to a single lipgloss renderer.
package theme

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Theme carries the styles and glyphs every component draws with.
type Theme struct {
	Header lipgloss.Style
	Label  lipgloss.Style
	Value  lipgloss.Style
	Muted  lipgloss.Style
	OK     lipgloss.Style
	Warn   lipgloss.Style
	Fail   lipgloss.Style
	Active lipgloss.Style

	states map[domain.ArrowState]stateGlyph
}

type stateGlyph struct {
	glyph string
	style func(Theme) lipgloss.Style
}

// New returns a Theme whose styles are bound to r.
func New(r *lipgloss.Renderer) Theme {
	return Theme{
		Header: r.NewStyle().Bold(true),
		Label:  r.NewStyle().Bold(true),
		Value:  r.NewStyle(),
		Muted:  r.NewStyle().Foreground(lipgloss.Color("8")),
		OK:     r.NewStyle().Foreground(lipgloss.Color("2")),
		Warn:   r.NewStyle().Foreground(lipgloss.Color("3")),
		Fail:   r.NewStyle().Foreground(lipgloss.Color("1")),
		Active: r.NewStyle().Foreground(lipgloss.Color("6")),
		states: stateTable(),
	}
}

// stateTable is a function rather than a package-level map because mutable
// package-level state is banned.
func stateTable() map[domain.ArrowState]stateGlyph {
	ok := func(t Theme) lipgloss.Style { return t.OK }
	muted := func(t Theme) lipgloss.Style { return t.Muted }
	active := func(t Theme) lipgloss.Style { return t.Active }
	warn := func(t Theme) lipgloss.Style { return t.Warn }

	return map[domain.ArrowState]stateGlyph{
		domain.ArrowStateAbsent:       {glyph: "○", style: muted},
		domain.ArrowStateReady:        {glyph: "●", style: ok},
		domain.ArrowStateRunning:      {glyph: "▶", style: active},
		domain.ArrowStateStopping:     {glyph: "◼", style: active},
		domain.ArrowStateDraining:     {glyph: "◍", style: active},
		domain.ArrowStateDetached:     {glyph: "◈", style: active},
		domain.ArrowStateInstalling:   {glyph: "⇣", style: active},
		domain.ArrowStateUninstalling: {glyph: "⇡", style: active},
		domain.ArrowStateUpdating:     {glyph: "⇅", style: active},
		domain.ArrowStateOutdated:     {glyph: "▲", style: warn},
		domain.ArrowStateRemoved:      {glyph: "✕", style: muted},
	}
}

// State renders s as a coloured glyph followed by its name.
func (t Theme) State(s domain.ArrowState) string {
	g, ok := t.states[s]
	if !ok {
		return t.Muted.Render("? " + string(s))
	}

	return g.style(t).Render(g.glyph + " " + string(s))
}
