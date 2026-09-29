package archive_test

import (
	"compress/bzip2"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
)

func TestArchive_Extract_CorruptCompressedStream(t *testing.T) {
	testCases := []struct {
		name  string
		file  string
		check func(t *testing.T, err error)
	}{
		{
			name:  "gzip",
			file:  "a.tar.gz",
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, gzip.ErrHeader) },
		},
		{
			name: "xz",
			file: "a.tar.xz",
		},
		{
			name: "bzip2",
			file: "a.tar.bz2",
			check: func(t *testing.T, err error) {
				var structural bzip2.StructuralError
				require.ErrorAs(t, err, &structural)
			},
		},
		{
			name:  "zstd",
			file:  "a.tar.zst",
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, zstd.ErrMagicMismatch) },
		},
		{
			name:  "single gzip",
			file:  "tool.gz",
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, gzip.ErrHeader) },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, tc.file, []byte("this is not a compressed stream at all"))
			to := filepath.Join(dir, "out")

			err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

			require.Error(t, err)
			if tc.check != nil {
				tc.check(t, err)
			}
			entries, rdErr := os.ReadDir(to)
			require.NoError(t, rdErr)
			assert.Empty(t, entries)
		})
	}
}
