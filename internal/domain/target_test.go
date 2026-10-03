package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTarget_ActivationFor(t *testing.T) {
	target := Target{Activation: map[string]string{"update": ActivationRestart}}

	testCases := []struct {
		name   string
		target Target
		method string
		want   string
	}{
		{name: "declared method", target: target, method: MethodUpdate, want: ActivationRestart},
		{name: "other method", target: target, method: MethodInstall, want: ""},
		{name: "no activation", target: Target{}, method: MethodUpdate, want: ""},
		{name: "unprefixed key is not a method", target: target, method: "update", want: ActivationRestart},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.target.ActivationFor(tc.method))
		})
	}
}

func TestActivationMethods_AreTheLifecycleMethodsThatStageSomething(t *testing.T) {
	assert.Equal(t, []string{"update"}, ActivationMethods())
}
