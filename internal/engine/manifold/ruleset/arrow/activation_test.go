package arrow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestActivationRule_Validate(t *testing.T) {
	testCases := []struct {
		name       string
		activation map[string]string
		wantRules  []string
	}{
		{name: "none"},
		{name: "update restart", activation: map[string]string{"update": "restart"}},
		{name: "install is not a method that stages", activation: map[string]string{"install": "restart"}, wantRules: []string{"invalid_activation_method"}},
		{name: "unknown method", activation: map[string]string{"execute": "restart"}, wantRules: []string{"invalid_activation_method"}},
		{name: "prefixed method is not a manifest key", activation: map[string]string{"_update": "restart"}, wantRules: []string{"invalid_activation_method"}},
		{name: "unknown value", activation: map[string]string{"update": "reboot"}, wantRules: []string{"invalid_activation_value"}},
		{name: "both wrong", activation: map[string]string{"stop": "reboot"}, wantRules: []string{"invalid_activation_method", "invalid_activation_value"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &domain.Arrow{Targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {Activation: tc.activation},
			}}

			errs := ActivationRule{}.Validate(m)

			var got []string
			for _, e := range errs {
				got = append(got, e.Rule)
			}
			require.ElementsMatch(t, tc.wantRules, got)
			if len(tc.wantRules) > 0 {
				assert.Contains(t, errs[0].Field, "targets[linux/amd64].activation")
			}
		})
	}
}

func TestActivationRule_Name(t *testing.T) {
	assert.Equal(t, "activation", ActivationRule{}.Name())
}
