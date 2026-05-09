package download_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	stepdownload "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/download"
)

func newTestHandler() wizstep.Handler[domainstep.FetchStep] {
	return stepdownload.NewHandler()
}

func TestHandler_Execute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "output.txt")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.NoError(t, err)
	data, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, "hello world", string(data))
}

func TestHandler_Execute_AbsolutePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "file.txt")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/other/dir"}, s)

	require.NoError(t, err)
	_, statErr := os.Stat(dst)
	assert.NoError(t, statErr, "file should be at absolute path, not joined with workDir")
}

func TestHandler_Execute_RelativePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	workDir := t.TempDir()
	s := domainstep.NewFetchStep("fetch", srv.URL, "file.txt", "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.NoError(t, err)
	expected := filepath.Join(workDir, "file.txt")
	_, statErr := os.Stat(expected)
	assert.NoError(t, statErr, "file should be at workDir/file.txt")
}

func TestHandler_Execute_VarExpansionInTo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	workDir := t.TempDir()
	s := domainstep.NewFetchStep("fetch", srv.URL, "${WORKDIR}/output.txt", "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{
		WorkDir: workDir,
		Vars:    map[string]string{"WORKDIR": workDir},
	}, s)

	require.NoError(t, err)
	expected := filepath.Join(workDir, "output.txt")
	_, statErr := os.Stat(expected)
	assert.NoError(t, statErr, "file should be written to the expanded WORKDIR path")
}

func TestHandler_Execute_DownloadError(t *testing.T) {
	h := newTestHandler()
	s := domainstep.NewFetchStep("fetch", "http://127.0.0.1:0/nonexistent", "/tmp/out.txt", "", "5s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.Error(t, err)
}

func TestHandler_Execute_Timeout(t *testing.T) {
	// Use a handler that blocks with context-aware timeout
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block for longer than the test timeout will allow
		select {
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Context cancelled - the download handler should have cancelled it
			http.Error(w, "context cancelled", http.StatusRequestTimeout)
		}
	}))
	defer slow.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "out.txt")
	s := domainstep.NewFetchStep("fetch", slow.URL, dst, "", "50ms", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.Error(t, err)
}
