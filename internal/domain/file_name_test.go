package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWindowsReservedName(t *testing.T) {
	testCases := []struct {
		name string
		want bool
	}{
		{name: "CON", want: true},
		{name: "prn", want: true},
		{name: "Aux", want: true},
		{name: "nul.exe", want: true},
		{name: "COM1", want: true},
		{name: "com9.txt", want: true},
		{name: "LPT1", want: true},
		{name: "lpt9", want: true},
		{name: "COM0"},
		{name: "LPT"},
		{name: "COM10"},
		{name: "console"},
		{name: "tool"},
		{name: "tool.con"},
		{name: "ABCD"},
		{name: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsWindowsReservedName(tc.name))
		})
	}
}
