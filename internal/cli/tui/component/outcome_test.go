package component_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
)

func TestOutcome_Render_SuccessAndFailure(t *testing.T) {
	th := newTestTheme(t)

	testCases := []struct {
		name   string
		result component.Result
		want   string
	}{
		{
			name:   "success",
			result: component.Result{OK: true, Subject: "github.com/u/r", Message: "installed"},
			want:   "✓ github.com/u/r  installed\n",
		},
		{
			name:   "failure",
			result: component.Result{OK: false, Subject: "github.com/u/r", Message: "build failed"},
			want:   "✗ github.com/u/r  build failed\n",
		},
		{
			name:   "no message",
			result: component.Result{OK: true, Subject: "daemon"},
			want:   "✓ daemon\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, component.Outcome(tc.result, th))
		})
	}
}
