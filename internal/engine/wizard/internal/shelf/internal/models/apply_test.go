package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCandidate_DisplayName(t *testing.T) {
	testCases := []struct {
		name string
		c    Candidate
		want string
	}{
		{name: "display wins", c: Candidate{Name: "tool", Display: "Tool"}, want: "Tool"},
		{name: "falls back to name", c: Candidate{Name: "tool"}, want: "tool"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.c.DisplayName())
		})
	}
}
