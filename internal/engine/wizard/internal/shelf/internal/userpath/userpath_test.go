package userpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains(t *testing.T) {
	testCases := []struct {
		name string
		list string
		dir  string
		sep  string
		fold bool
		want bool
	}{
		{name: "exact", list: "/a:/b", dir: "/b", sep: ":", want: true},
		{name: "cleaned", list: "/a:/b/", dir: "/b", sep: ":", want: true},
		{name: "case sensitive", list: "/a:/B", dir: "/b", sep: ":", want: false},
		{name: "case folded", list: `C:\A;C:\B`, dir: `c:\b`, sep: ";", fold: true, want: true},
		{name: "empty entries", list: "::", dir: "/b", sep: ":", want: false},
		{name: "empty list", list: "", dir: "/b", sep: ";", fold: true, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Contains(tc.list, tc.dir, tc.sep, tc.fold))
		})
	}
}
