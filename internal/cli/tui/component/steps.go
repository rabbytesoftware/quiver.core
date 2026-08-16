package component

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// StepState is the execution state of a single lifecycle step.
type StepState int

const (
	// StepPending is a step that has not started.
	StepPending StepState = iota
	// StepRunning is the step currently executing.
	StepRunning
	// StepDone is a step that completed successfully.
	StepDone
	// StepFailed is a step that terminated in failure.
	StepFailed
)

// Step is one entry in a lifecycle run.
type Step struct {
	Name  string
	State StepState
}

// Steps renders an ordered, numbered step list.
func Steps(steps []Step, t theme.Theme) string {
	var b strings.Builder

	for i, s := range steps {
		b.WriteString(fmt.Sprintf("%s %d of %d  %s\n",
			stepGlyph(s.State, t), i+1, len(steps), s.Name))
	}

	return b.String()
}

// stepGlyph covers every StepState with no default, which is what the
// exhaustive linter requires. The trailing return is unreachable for valid
// values and exists only to satisfy the compiler.
func stepGlyph(s StepState, t theme.Theme) string {
	switch s {
	case StepPending:
		return t.Muted.Render("·")
	case StepRunning:
		return t.Active.Render("▸")
	case StepDone:
		return t.OK.Render("✓")
	case StepFailed:
		return t.Fail.Render("✗")
	}

	return t.Muted.Render("·")
}
