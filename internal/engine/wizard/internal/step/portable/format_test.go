package portable

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsExecutable_Magic(t *testing.T) {
	testCases := []struct {
		name string
		head string
		want bool
	}{
		{name: "elf", head: "\x7fELF\x02", want: true},
		{name: "mach-o big endian", head: "\xfe\xed\xfa\xcf", want: true},
		{name: "mach-o little endian", head: "\xcf\xfa\xed\xfe", want: true},
		{name: "mach-o universal", head: "\xca\xfe\xba\xbe", want: true},
		{name: "pe exactly two bytes", head: "MZ", want: true},
		{name: "truncated elf", head: "\x7fEL", want: false},
		{name: "empty", head: "", want: false},
		{name: "text", head: "hello world", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isExecutable(bytes.NewReader([]byte(tc.head))))
		})
	}
}
