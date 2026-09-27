package unpack_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func TestArchive_Extract_CorruptCompressedStream(t *testing.T) {
	testCases := []struct {
		name string
		file string
	}{
		{name: "gzip", file: "a.tar.gz"},
		{name: "xz", file: "a.tar.xz"},
		{name: "bzip2", file: "a.tar.bz2"},
		{name: "zstd", file: "a.tar.zst"},
		{name: "single gzip", file: "tool.gz"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, tc.file, []byte("this is not a compressed stream at all"))

			err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "unpack")
		})
	}
}
