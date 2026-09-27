package extract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Execute_SingleFileStripsExtension(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		build    func(t *testing.T) []byte
		wantFile string
		wantBody string
	}{
		{
			name:     "gzip",
			file:     "tool.gz",
			build:    func(t *testing.T) []byte { return gzipBytes(t, []byte("gz tool")) },
			wantFile: "tool",
			wantBody: "gz tool",
		},
		{
			name:     "xz",
			file:     "tool-linux.xz",
			build:    func(t *testing.T) []byte { return xzBytes(t, []byte("xz tool")) },
			wantFile: "tool-linux",
			wantBody: "xz tool",
		},
		{
			name:     "zstd",
			file:     "tool.v2.zst",
			build:    func(t *testing.T) []byte { return zstdBytes(t, []byte("zst tool")) },
			wantFile: "tool.v2",
			wantBody: "zst tool",
		},
		{
			name:     "bzip2",
			file:     "plain.bz2",
			build:    func(_ *testing.T) []byte { return bzip2PlainFixture() },
			wantFile: "plain",
			wantBody: "plain bzip2\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, tc.file, tc.build(t))
			to := filepath.Join(dir, "bin")

			require.NoError(t, runExtract(t, testMaxBytes, from, to, ""))

			out := filepath.Join(to, tc.wantFile)
			assert.Equal(t, tc.wantBody, readString(t, out))
			info, err := os.Stat(out)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		})
	}
}

func TestHandler_Execute_SingleFileWithoutStem(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, ".gz", gzipBytes(t, []byte("x")))

	err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entry name")
}
