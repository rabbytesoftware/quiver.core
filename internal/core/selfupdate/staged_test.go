package selfupdate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
)

func writeStaged(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
	return path
}

func TestFingerprint_MeasuresTheFile(t *testing.T) {
	path := writeStaged(t, "binary bytes")
	sum := sha256.Sum256([]byte("binary bytes"))

	size, digest, err := selfupdate.Fingerprint(context.Background(), path)

	require.NoError(t, err)
	assert.EqualValues(t, len("binary bytes"), size)
	assert.Equal(t, hex.EncodeToString(sum[:]), digest)
}

func TestFingerprint_MissingFile(t *testing.T) {
	_, _, err := selfupdate.Fingerprint(context.Background(), filepath.Join(t.TempDir(), "gone"))

	require.Error(t, err)
}

func TestVerify(t *testing.T) {
	path := writeStaged(t, "binary bytes")
	size, digest, err := selfupdate.Fingerprint(context.Background(), path)
	require.NoError(t, err)

	testCases := []struct {
		name    string
		path    string
		size    int64
		digest  string
		wantErr bool
	}{
		{name: "intact", path: path, size: size, digest: digest},
		{name: "case of the digest does not matter", path: path, size: size, digest: "  " + upper(digest)},
		{name: "truncated", path: path, size: size + 1, digest: digest, wantErr: true},
		{name: "replaced", path: path, size: size, digest: "00" + digest[2:], wantErr: true},
		{name: "missing", path: filepath.Join(t.TempDir(), "gone"), size: size, digest: digest, wantErr: true},
		{name: "nothing recorded to compare with", path: path, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := selfupdate.Verify(context.Background(), tc.path, tc.size, tc.digest)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestVerify_EmptyFileIsNeverUsable(t *testing.T) {
	path := writeStaged(t, "")
	size, digest, err := selfupdate.Fingerprint(context.Background(), path)
	require.NoError(t, err)

	require.Error(t, selfupdate.Verify(context.Background(), path, size, digest))
}

func upper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'f' {
			out[i] = c - 32
		}
	}
	return string(out)
}
