package dmg_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/dmg"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
)

func TestIs_Detection(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "koly trailer", data: unpacktest.DmgTrailer(), want: true},
		{name: "no trailer", data: make([]byte, 1024), want: false},
		{name: "too short", data: []byte("short"), want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "image.bin", tc.data)
			src, err := os.Open(from)
			require.NoError(t, err)
			defer src.Close()
			info, err := src.Stat()
			require.NoError(t, err)

			assert.Equal(t, tc.want, dmg.Is(src, info.Size()))
		})
	}
}
