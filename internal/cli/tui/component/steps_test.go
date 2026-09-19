package component_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
)

func TestSteps_Render_NumbersStepsAndMarksState(t *testing.T) {
	th := newTestTheme(t)
	steps := []component.Step{
		{Name: "fetch source", State: component.StepDone},
		{Name: "build", State: component.StepRunning},
		{Name: "link", State: component.StepPending},
	}

	got := component.Steps(steps, th)

	assert.Equal(t, ""+
		"✓ 1 of 3  fetch source\n"+
		"▸ 2 of 3  build\n"+
		"· 3 of 3  link\n", got)
}

func TestSteps_Render_FailedStepIsMarked(t *testing.T) {
	th := newTestTheme(t)

	got := component.Steps([]component.Step{
		{Name: "build", State: component.StepFailed},
	}, th)

	assert.Equal(t, "✗ 1 of 1  build\n", got)
}

func TestSteps_Render_EmptyIsEmpty(t *testing.T) {
	assert.Equal(t, "", component.Steps(nil, newTestTheme(t)))
}

func TestSteps_Render_UnknownStateFallsBackToPending(t *testing.T) {
	th := newTestTheme(t)

	got := component.Steps([]component.Step{
		{Name: "odd", State: component.StepState(99)},
	}, th)

	assert.Equal(t, "· 1 of 1  odd\n", got)
}
