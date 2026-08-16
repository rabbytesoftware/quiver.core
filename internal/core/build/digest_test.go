package build

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
)

func TestDigest_ExistingFile_ReturnsSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quiver")
	require.NoError(t, fns.Write(context.Background(), path, []byte("candidate")))

	got, err := Digest(context.Background(), path)

	require.NoError(t, err)
	want := sha256.Sum256([]byte("candidate"))
	assert.Equal(t, fmt.Sprintf("%x", want), got)
}

func TestDigest_MissingFile_ReturnsError(t *testing.T) {
	got, err := Digest(context.Background(), filepath.Join(t.TempDir(), "missing"))

	require.Error(t, err)
	assert.Empty(t, got)
}

func TestDigest_CancelledContext_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quiver")
	require.NoError(t, fns.Write(context.Background(), path, []byte("candidate")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := Digest(ctx, path)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, got)
}

func TestDigestReader_ReadFailureReturnsError(t *testing.T) {
	wantErr := errors.New("read failed")
	reader := &stubReadCloser{reader: errorReader{err: wantErr}}

	got, err := digestReader(context.Background(), reader)

	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, got)
	assert.True(t, reader.closed)
}

func TestDigestReader_CloseFailureReturnsError(t *testing.T) {
	wantErr := errors.New("close failed")
	reader := &stubReadCloser{reader: bytes.NewReader([]byte("candidate")), closeErr: wantErr}

	got, err := digestReader(context.Background(), reader)

	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, got)
}

type stubReadCloser struct {
	reader   io.Reader
	closeErr error
	closed   bool
}

func (r *stubReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *stubReadCloser) Close() error {
	r.closed = true
	return r.closeErr
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
