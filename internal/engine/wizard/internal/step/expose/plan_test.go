package expose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestPlan(t *testing.T) {
	expose := domain.Expose{
		CLI:     []domain.ExposeEntry{{Name: "tool", Path: "bin/tool"}},
		Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "auto", Icon: "icon.png", Categories: []string{"Development"}}},
	}
	run := domainstep.NewRunStep("test", "echo hi", false, "", true)
	lifecycle := []domainstep.Step{run}
	desktop := domainstep.NewExposeStep("desktop", "Tool", "auto")
	desktop.Icon = "icon.png"
	desktop.Categories = []string{"Development"}
	desktop.MediaIcon = "/media.png"
	cli := domainstep.NewExposeStep("cli", "tool", "bin/tool")
	cli.MediaIcon = "/media.png"

	testCases := []struct {
		name   string
		method string
		expose domain.Expose
		want   []domainstep.Step
	}{
		{name: "install exposes after its steps, desktop first", method: domain.MethodInstall, expose: expose, want: []domainstep.Step{run, desktop, cli}},
		{name: "update exposes after its steps", method: domain.MethodUpdate, expose: expose, want: []domainstep.Step{run, desktop, cli}},
		{name: "uninstall removes before its steps", method: domain.MethodUninstall, expose: expose, want: []domainstep.Step{domainstep.NewUnexposeStep(), run}},
		{name: "execute is untouched", method: domain.MethodExecute, expose: expose, want: lifecycle},
		{name: "nothing to expose is untouched", method: domain.MethodInstall, want: lifecycle},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Plan(tc.method, tc.expose, "/media.png", lifecycle)

			assert.Equal(t, tc.want, got)
			assert.Equal(t, []domainstep.Step{run}, lifecycle)
		})
	}
}
