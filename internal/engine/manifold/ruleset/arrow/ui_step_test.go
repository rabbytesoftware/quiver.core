package arrow

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func arrowWithExecute(steps ...step.Step) *domain.Arrow {
	return &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			"linux/amd64": {Lifecycle: domain.TargetLifecycle{Execute: step.StepList(steps)}},
		},
	}
}

func uiRun(cmd string) step.RunStep { return step.NewRunStep("serve", cmd, false, "10s", true) }

func TestUIStepRule(t *testing.T) {
	testCases := []struct {
		name    string
		arrow   *domain.Arrow
		wantMsg string
	}{
		{
			name: "listen then run is valid",
			arrow: arrowWithExecute(
				step.NewUIStep("Chat", []string{"unix"}, "", "/", true),
				uiRun("./chat --listen ${ARROW_UI_LISTEN}"),
			),
		},
		{
			name:  "static is valid",
			arrow: arrowWithExecute(step.NewUIStep("Docs", nil, "./dist", "", true)),
		},
		{
			name:    "both sources",
			arrow:   arrowWithExecute(step.NewUIStep("x", []string{"unix"}, "./d", "", true)),
			wantMsg: "exactly one of listen or static is required",
		},
		{
			name:    "no source",
			arrow:   arrowWithExecute(step.NewUIStep("x", nil, "", "", true)),
			wantMsg: "exactly one of listen or static is required",
		},
		{
			name:    "unknown listen kind",
			arrow:   arrowWithExecute(step.NewUIStep("x", []string{"tcp"}, "", "", true)),
			wantMsg: `unknown listen kind "tcp"`,
		},
		{
			name:    "pipe only is not provisioned",
			arrow:   arrowWithExecute(step.NewUIStep("x", []string{"pipe"}, "", "", true)),
			wantMsg: "pipe is not provisioned in v0; include unix",
		},
		{
			name:  "unix is valid",
			arrow: arrowWithExecute(step.NewUIStep("x", []string{"unix"}, "", "", true)),
		},
		{
			name:  "unix and pipe is valid",
			arrow: arrowWithExecute(step.NewUIStep("x", []string{"unix", "pipe"}, "", "", true)),
		},
		{
			name:    "static escapes",
			arrow:   arrowWithExecute(step.NewUIStep("x", nil, "../etc", "", true)),
			wantMsg: "static must be a relative path",
		},
		{
			name:    "static absolute",
			arrow:   arrowWithExecute(step.NewUIStep("x", nil, "/etc", "", true)),
			wantMsg: "static must be a relative path",
		},
		{
			name:    "path without slash",
			arrow:   arrowWithExecute(step.NewUIStep("x", nil, "./d", "app", true)),
			wantMsg: "path must start with /",
		},
		{
			name: "two ui steps",
			arrow: arrowWithExecute(
				step.NewUIStep("a", nil, "./d", "", true),
				step.NewUIStep("b", nil, "./d", "", true),
			),
			wantMsg: "at most one ui step",
		},
		{
			name: "run before ui listen",
			arrow: arrowWithExecute(
				uiRun("./chat --listen ${ARROW_UI_LISTEN}"),
				step.NewUIStep("Chat", []string{"unix"}, "", "", true),
			),
			wantMsg: "${ARROW_UI_LISTEN} is only available after a ui step with listen",
		},
		{
			name:    "variable without ui step",
			arrow:   arrowWithExecute(uiRun("./chat --listen ${ARROW_UI_LISTEN}")),
			wantMsg: "${ARROW_UI_LISTEN} is only available after a ui step with listen",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := UIStepRule{}.Validate(tc.arrow)
			if tc.wantMsg == "" {
				require.Empty(t, errs)
				return
			}
			require.NotEmpty(t, errs)
			require.Contains(t, errs[0].Message, tc.wantMsg)
			require.Equal(t, "ui_step", errs[0].Rule)
			require.Contains(t, errs[0].Field, "targets[linux/amd64].lifecycle.execute[")
		})
	}
}

func TestUIStepRule_CustomMethod(t *testing.T) {
	a := &domain.Arrow{Targets: map[domain.OS]domain.Target{
		"linux/amd64": {Methods: map[string]domain.Method{
			"open": {Steps: step.StepList{step.NewUIStep("x", nil, "", "", true)}},
		}},
	}}
	errs := UIStepRule{}.Validate(a)
	require.NotEmpty(t, errs)
	require.Contains(t, errs[0].Field, "targets[linux/amd64].methods.open.steps[0]")
}

func TestVariableRefs_KnowsArrowUIListen(t *testing.T) {
	a := arrowWithExecute(
		step.NewUIStep("Chat", []string{"unix"}, "", "", true),
		uiRun("./chat --listen ${ARROW_UI_LISTEN}"),
	)
	for _, e := range (VariableRefsRule{}).Validate(a) {
		require.NotEqual(t, "unresolved_variable", e.Rule, e.Message)
	}
}

func TestVariableRefs_PreinstalledRejectsArrowUIListen(t *testing.T) {
	a := &domain.Arrow{Targets: map[domain.OS]domain.Target{
		"linux/amd64": {Lifecycle: domain.TargetLifecycle{
			Preinstalled: step.StepList{uiRun("probe ${ARROW_UI_LISTEN}")},
		}},
	}}
	errs := VariableRefsRule{}.Validate(a)
	require.NotEmpty(t, errs)
	require.Equal(t, "unresolved_variable", errs[0].Rule)
}
