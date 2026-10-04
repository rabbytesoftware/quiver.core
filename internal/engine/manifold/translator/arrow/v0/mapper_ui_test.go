package v0

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestToStep_UI(t *testing.T) {
	cases := []struct {
		name string
		in   stepV0
		want step.UIStep
	}{
		{
			name: "listen",
			in:   stepV0{Type: "ui", Title: "Chat", Listen: []string{"unix"}, Path: "/"},
			want: step.NewUIStep("Chat", []string{"unix"}, "", "/", true),
		},
		{
			name: "static",
			in:   stepV0{Type: "ui", Title: "Docs", Static: "./dist"},
			want: step.NewUIStep("Docs", nil, "./dist", "", true),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toStep(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
