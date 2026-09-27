package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExposeKind_Values(t *testing.T) {
	testCases := []struct {
		name string
		kind ExposeKind
		want string
	}{
		{
			name: "cli",
			kind: ExposeKindCLI,
			want: "cli",
		},
		{
			name: "desktop",
			kind: ExposeKindDesktop,
			want: "desktop",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, string(tc.kind))
		})
	}
}
