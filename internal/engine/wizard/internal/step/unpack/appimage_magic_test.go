package unpack_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
)

func TestIsAppImage_Detection(t *testing.T) {
	testCases := []struct {
		name string
		head []byte
		want bool
	}{
		{"type2", append([]byte("\x7fELF\x02\x01\x01\x00"), []byte("AI\x02")...), true},
		{"type1", append([]byte("\x7fELF\x02\x01\x01\x00"), []byte("AI\x01")...), true},
		{"plain elf", []byte("\x7fELF\x02\x01\x01\x00\x00\x00\x00"), false},
		{"zip", []byte("PK\x03\x04\x00\x00\x00\x00\x00\x00\x00"), false},
		{"short", []byte("\x7fELF"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := unpack.IsAppImage(bytes.NewReader(tc.head))

			assert.Equal(t, tc.want, got)
		})
	}
}
