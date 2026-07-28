package lifecycle_test

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/lifecycle"
)

func modelWith(events ...apidto.ArrowRuntimeDTO) lifecycle.Model {
	m := lifecycle.NewModel("install", "github.com/user/a")
	for _, evt := range events {
		next, _ := m.Update(lifecycle.EventMsg(evt))
		m = next.(lifecycle.Model)
	}
	return m
}

func TestModel_ViewShowsHeaderAndCounter(t *testing.T) {
	m := modelWith(rt("installing", &apidto.RunRecordDTO{
		Method: "_install",
		Steps: []apidto.StepProgressDTO{
			step(0, "completed", "Resolving manifest"),
			step(1, "running", "Fetching"),
			step(2, "pending", ""),
		},
	}, nil))

	view := m.View()
	assert.Contains(t, view, "▸ quiver")
	assert.Contains(t, view, "install")
	assert.Contains(t, view, "github.com/user/a")
	assert.Contains(t, view, "1 of 3")
}

func TestModel_ViewRendersStepIcons(t *testing.T) {
	m := modelWith(rt("installing", &apidto.RunRecordDTO{
		Method: "_install",
		Steps: []apidto.StepProgressDTO{
			step(0, "completed", "Resolving manifest"),
			step(1, "running", "Fetching"),
			step(2, "pending", "Configuring"),
		},
	}, nil))

	view := m.View()
	assert.Contains(t, view, "✓")
	assert.Contains(t, view, "○")
	assert.Contains(t, view, "1  ✓")
	assert.Contains(t, view, "Resolving manifest")
}

func TestModel_UntitledStepPlaceholder(t *testing.T) {
	m := modelWith(rt("installing", &apidto.RunRecordDTO{
		Method: "_install",
		Steps:  []apidto.StepProgressDTO{step(0, "running", "")},
	}, nil))

	assert.Contains(t, m.View(), "[Untitled step]")
}

func TestModel_TerminalEventQuitsWithSuccessBox(t *testing.T) {
	m := lifecycle.NewModel("install", "github.com/user/a")
	next, cmd := m.Update(lifecycle.EventMsg(rt("ready", nil,
		&apidto.ReturnDTO{Method: "_install", Outcome: "success"})))
	m = next.(lifecycle.Model)

	assert.NotNil(t, cmd, "terminal event should produce a quit command")
	view := m.View()
	assert.Contains(t, view, "✓")
	assert.Contains(t, view, "ready")
	assert.True(t, m.Done())
	assert.Equal(t, "success", m.Result().Outcome)
}

func TestModel_FailureBoxNamesFailedStep(t *testing.T) {
	msg := "fetch: 404 not found"
	failed := step(0, "failed", "Downloading")
	failed.Error = &msg
	m := lifecycle.NewModel("install", "github.com/user/a")
	next, _ := m.Update(lifecycle.EventMsg(rt("absent", nil, &apidto.ReturnDTO{
		Method:  "_install",
		Outcome: "failed",
		Steps:   []apidto.StepProgressDTO{failed},
	})))
	m = next.(lifecycle.Model)

	view := m.View()
	assert.Contains(t, view, "✗")
	assert.Contains(t, view, "failed")
	assert.Contains(t, view, "fetch: 404 not found")
}

func TestModel_TickAdvancesSpinnerAndElapsed(t *testing.T) {
	m := lifecycle.NewModel("install", "github.com/user/a")
	next, cmd := m.Update(lifecycle.TickMsg{})
	m = next.(lifecycle.Model)

	assert.NotNil(t, cmd, "tick should schedule the next tick")
	assert.True(t, strings.Contains(m.View(), "s"), "view should show elapsed seconds")
}

func TestModel_CtrlCQuits(t *testing.T) {
	m := lifecycle.NewModel("install", "github.com/user/a")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
}

func TestModel_InitSchedulesTick(t *testing.T) {
	m := lifecycle.NewModel("install", "github.com/user/a")
	assert.NotNil(t, m.Init())
}

func TestModel_PastTenseAllOps(t *testing.T) {
	testCases := []struct {
		op       string
		recorded string
		want     string
	}{
		{"install", "_install", "Installed successfully"},
		{"uninstall", "_uninstall", "Uninstalled successfully"},
		{"update", "_update", "Updated successfully"},
		{"stop", "_stop", "Stopped successfully"},
		{"run", "_execute", "Runtime started"},
		{"backup", "backup", "backup completed"},
	}
	for _, tc := range testCases {
		t.Run(tc.op, func(t *testing.T) {
			m := lifecycle.NewModel(tc.op, "github.com/user/a")
			next, _ := m.Update(lifecycle.EventMsg(rt("ready", nil,
				&apidto.ReturnDTO{Method: tc.recorded, Outcome: "success"})))
			assert.Contains(t, next.(lifecycle.Model).View(), tc.want)
		})
	}
}

func TestModel_TickAfterDoneStops(t *testing.T) {
	m := lifecycle.NewModel("install", "github.com/user/a")
	next, _ := m.Update(lifecycle.EventMsg(rt("ready", nil,
		&apidto.ReturnDTO{Method: "_install", Outcome: "success"})))
	m = next.(lifecycle.Model)

	_, cmd := m.Update(lifecycle.TickMsg{})
	assert.Nil(t, cmd, "no further ticks once done")
}

func TestModel_OtherKeysIgnored(t *testing.T) {
	m := lifecycle.NewModel("install", "github.com/user/a")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	assert.Nil(t, cmd)
}

func TestModel_FailedStepErrorNotDuplicated(t *testing.T) {
	msg := "unique-error-marker-xyz"
	ok := step(0, "completed", "Resolve dependencies")
	failed := step(1, "failed", "Download Chrome")
	failed.Error = &msg

	m := lifecycle.NewModel("install", "github.com/user/a")
	next, _ := m.Update(lifecycle.EventMsg(rt("absent", nil, &apidto.ReturnDTO{
		Method:  "_install",
		Outcome: "failed",
		Steps:   []apidto.StepProgressDTO{ok, failed},
	})))
	m = next.(lifecycle.Model)

	view := m.View()
	assert.Equal(t, 1, strings.Count(view, msg),
		"the error must appear only in the completion box, not also inline on the step row")
}

func TestModel_LongErrorWrapsToWindowWidth(t *testing.T) {
	long := "Download https://github.com/example/releases/latest/download/" +
		"some-very-long-asset-name-x86_64.AppImage: GetInfo: HTTP 404"
	failed := step(0, "failed", "Download Discord")
	failed.Error = &long

	m := lifecycle.NewModel("install", "github.com/user/a")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60})
	m = next.(lifecycle.Model)
	next, _ = m.Update(lifecycle.EventMsg(rt("absent", nil, &apidto.ReturnDTO{
		Method:  "_install",
		Outcome: "failed",
		Steps:   []apidto.StepProgressDTO{failed},
	})))
	m = next.(lifecycle.Model)

	view := m.View()
	assert.Contains(t, view, "HTTP 404", "the tail of the error must survive wrapping")
	for _, line := range strings.Split(view, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 60,
			"no rendered line may exceed the window width: %q", line)
	}
}

func TestPlainPrinter_IgnoresEventsWithoutActiveRun(t *testing.T) {
	var buf bytes.Buffer
	p := lifecycle.NewPlainPrinter(&buf)
	p.Observe(rt("ready", nil, nil))
	assert.Empty(t, buf.String())
}
