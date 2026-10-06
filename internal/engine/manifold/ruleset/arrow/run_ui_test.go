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

func uiRunWith(
	cmd string,
	ui step.UIOptions,
) step.RunStep {
	return uiRun(cmd).WithUI(ui)
}

func TestRunUIRule(t *testing.T) {
	testCases := []struct {
		name    string
		arrow   *domain.Arrow
		wantMsg string
	}{
		{
			name: "listen with the variable is valid",
			arrow: arrowWithExecute(
				uiRunWith("./chat --listen ${ARROW_UI_LISTEN}", step.UIOptions{Title: "Chat", Listen: []string{"unix"}, Path: "/"}),
			),
		},
		{
			name: "default listen with the variable is valid",
			arrow: arrowWithExecute(
				uiRunWith("./chat --listen ${ARROW_UI_LISTEN}", step.UIOptions{}),
			),
		},
		{
			name:  "static is valid without the variable",
			arrow: arrowWithExecute(uiRunWith("./keepalive", step.UIOptions{Static: "./dist"})),
		},
		{
			name:  "run without ui is valid",
			arrow: arrowWithExecute(uiRun("./chat")),
		},
		{
			name:    "listen and static",
			arrow:   arrowWithExecute(uiRunWith("./c", step.UIOptions{Listen: []string{"unix"}, Static: "./d"})),
			wantMsg: "listen and static are mutually exclusive",
		},
		{
			name:    "unknown listen kind",
			arrow:   arrowWithExecute(uiRunWith("./c", step.UIOptions{Listen: []string{"tcp"}})),
			wantMsg: `unknown listen kind "tcp"`,
		},
		{
			name:    "pipe only is not provisioned",
			arrow:   arrowWithExecute(uiRunWith("./c", step.UIOptions{Listen: []string{"pipe"}})),
			wantMsg: "pipe is not provisioned in v0; include unix",
		},
		{
			name:  "unix and pipe is valid",
			arrow: arrowWithExecute(uiRunWith("./c", step.UIOptions{Listen: []string{"unix", "pipe"}})),
		},
		{
			name:    "static escapes",
			arrow:   arrowWithExecute(uiRunWith("./c", step.UIOptions{Static: "../etc"})),
			wantMsg: "static must be a relative path",
		},
		{
			name:    "static absolute",
			arrow:   arrowWithExecute(uiRunWith("./c", step.UIOptions{Static: "/etc"})),
			wantMsg: "static must be a relative path",
		},
		{
			name:    "path without slash",
			arrow:   arrowWithExecute(uiRunWith("./c", step.UIOptions{Static: "./d", Path: "app"})),
			wantMsg: "path must start with /",
		},
		{
			name:    "variable in a run without ui",
			arrow:   arrowWithExecute(uiRun("./chat --listen ${ARROW_UI_LISTEN}")),
			wantMsg: "${ARROW_UI_LISTEN} is only available in the command of a run whose ui listens",
		},
		{
			name: "variable in another run",
			arrow: arrowWithExecute(
				uiRunWith("./chat", step.UIOptions{}),
				uiRun("./other --listen ${ARROW_UI_LISTEN}"),
			),
			wantMsg: "${ARROW_UI_LISTEN} is only available in the command of a run whose ui listens",
		},
		{
			name:    "variable in a static ui run",
			arrow:   arrowWithExecute(uiRunWith("./k ${ARROW_UI_LISTEN}", step.UIOptions{Static: "./d"})),
			wantMsg: "${ARROW_UI_LISTEN} is only available in the command of a run whose ui listens",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := RunUIRule{}.Validate(tc.arrow)
			if tc.wantMsg == "" {
				require.Empty(t, errs)
				return
			}
			require.NotEmpty(t, errs)
			require.Contains(t, errs[0].Message, tc.wantMsg)
			require.Equal(t, "run_ui", errs[0].Rule)
			require.Contains(t, errs[0].Field, "targets[linux/amd64].lifecycle.execute[")
		})
	}
}

func TestRunUIRule_EachRunMayHaveItsOwnUI(t *testing.T) {
	a := arrowWithExecute(
		uiRunWith("./a --listen ${ARROW_UI_LISTEN}", step.UIOptions{}),
		uiRunWith("./b", step.UIOptions{Static: "./d"}),
	)
	require.Empty(t, RunUIRule{}.Validate(a))
}

func TestRunUIRule_CustomMethod(t *testing.T) {
	a := &domain.Arrow{Targets: map[domain.OS]domain.Target{
		"linux/amd64": {Methods: map[string]domain.Method{
			"open": {Steps: step.StepList{uiRunWith("./c", step.UIOptions{Static: "../x"})}},
		}},
	}}
	errs := RunUIRule{}.Validate(a)
	require.NotEmpty(t, errs)
	require.Contains(t, errs[0].Field, "targets[linux/amd64].methods.open.steps[0]")
}

func TestVariableRefs_KnowsArrowUIListen(t *testing.T) {
	a := arrowWithExecute(
		uiRunWith("./chat --listen ${ARROW_UI_LISTEN}", step.UIOptions{}),
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

func TestRunUIRule_RejectsUIInPreinstalled(t *testing.T) {
	a := &domain.Arrow{Targets: map[domain.OS]domain.Target{
		"linux/amd64": {Lifecycle: domain.TargetLifecycle{
			Preinstalled: step.StepList{uiRunWith("probe", step.UIOptions{Static: "./dist"})},
		}},
	}}
	errs := RunUIRule{}.Validate(a)
	require.Len(t, errs, 1)
	require.Equal(t, "run_ui", errs[0].Rule)
	require.Equal(t, "targets[linux/amd64].lifecycle.preinstalled[0]", errs[0].Field)
	require.Contains(t, errs[0].Message, "ui is not allowed in preinstalled")
}
