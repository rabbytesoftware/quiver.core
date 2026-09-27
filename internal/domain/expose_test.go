package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpose_IsEmpty(t *testing.T) {
	testCases := []struct {
		name   string
		expose Expose
		want   bool
	}{
		{
			name:   "zero value",
			expose: Expose{},
			want:   true,
		},
		{
			name:   "empty slices",
			expose: Expose{CLI: []ExposeEntry{}, Desktop: []ExposeEntry{}},
			want:   true,
		},
		{
			name:   "has cli entry",
			expose: Expose{CLI: []ExposeEntry{{Name: "mytool", Path: ExposeAuto}}},
			want:   false,
		},
		{
			name:   "has desktop entry",
			expose: Expose{Desktop: []ExposeEntry{{Name: "mytool", Path: ExposeAuto}}},
			want:   false,
		},
		{
			name: "has both",
			expose: Expose{
				CLI:     []ExposeEntry{{Name: "mytool", Path: ExposeAuto}},
				Desktop: []ExposeEntry{{Name: "mytool", Path: ExposeAuto}},
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.expose.IsEmpty())
		})
	}
}

func TestExposeAuto_Value(t *testing.T) {
	assert.Equal(t, "auto", ExposeAuto)
}
