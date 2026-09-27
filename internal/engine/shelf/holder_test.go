package shelf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHolder_Refusal(t *testing.T) {
	testCases := []struct {
		name string
		h    holder
		want string
	}{
		{name: "absent", h: holder{}, want: ""},
		{name: "same owner", h: holder{exists: true, namespace: bareA}, want: ""},
		{name: "unmanaged", h: holder{exists: true}, want: reasonUnmanaged},
		{name: "foreign", h: holder{exists: true, namespace: bareB}, want: "owned by github.com/other/thing"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.h.refusal(bareA))
		})
	}
}
