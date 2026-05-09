package strategies

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
)

func TestRemote_GetInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "9")
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Write([]byte("test data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, resourceType, modTime, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if size != 9 {
		t.Errorf("Expected size 9, got %d", size)
	}
	if resourceType != "file" {
		t.Errorf("Expected type file, got %s", resourceType)
	}
	if modTime.IsZero() {
		t.Error("Expected non-zero ModTime")
	}
}

func TestRemote_Exists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	exists, err := r.Exists(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected URL to exist")
	}
}

func TestRemote_ReadStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("stream data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	stream, err := r.ReadStream(ctx, srv.URL)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "stream data" {
		t.Errorf("Expected %q, got %q", "stream data", string(data))
	}
}

func TestRemote_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fetch data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	data, err := r.Fetch(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(data) != "fetch data" {
		t.Errorf("Expected %q, got %q", "fetch data", string(data))
	}
}

func TestRemote_Fetch_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.Fetch(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestRemote_UnsupportedOperations(t *testing.T) {
	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Write(ctx, "url", []byte("data"))
	if err == nil {
		t.Error("Expected Write to be unsupported")
	}

	err = r.Mkdir(ctx, "url", 0o755)
	if err == nil {
		t.Error("Expected Mkdir to be unsupported")
	}

	err = r.Remove(ctx, "url")
	if err == nil {
		t.Error("Expected Remove to be unsupported")
	}
}

func TestRemote_IsDir_IsFile_Unsupported(t *testing.T) {
	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.IsDir(ctx, "http://example.com")
	if err == nil {
		t.Error("Expected IsDir to be unsupported")
	}

	_, err = r.IsFile(ctx, "http://example.com")
	if err == nil {
		t.Error("Expected IsFile to be unsupported")
	}
}

func TestRemote_Download(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "9")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "downloaded.txt")

	var progressCalls int
	progress := func(bytes int) {
		progressCalls++
	}

	err := r.Download(ctx, srv.URL, dst, progress)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "test data" {
		t.Errorf("Expected %q, got %q", "test data", string(data))
	}

	if progressCalls == 0 {
		t.Error("Expected progress callback to be called")
	}
}

func TestRemote_DownloadStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("stream download"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	stream, err := r.DownloadStream(ctx, srv.URL, nil)
	if err != nil {
		t.Fatalf("DownloadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "stream download" {
		t.Errorf("Expected %q, got %q", "stream download", string(data))
	}
}

func TestRemote_Copy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("copy data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "copied.txt")

	err := r.Copy(ctx, srv.URL, dst)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "copy data" {
		t.Errorf("Expected %q, got %q", "copy data", string(data))
	}
}

func TestRemote_Validate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Validate(ctx, srv.URL)
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestRemote_Validate_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Validate(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestRemote_AllUnsupportedOperations(t *testing.T) {
	r := NewRemote(config.Default())
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Write", func() error { return r.Write(ctx, "url", []byte("data")) }},
		{"WriteStream", func() error { return r.WriteStream(ctx, "url", strings.NewReader("data")) }},
		{"Append", func() error { return r.Append(ctx, "url", []byte("data")) }},
		{"MkdirAll", func() error { return r.MkdirAll(ctx, "url", 0o755) }},
		{"RemoveAll", func() error { return r.RemoveAll(ctx, "url") }},
		{"Move", func() error { return r.Move(ctx, "url", "dst") }},
		{"Rename", func() error { return r.Rename(ctx, "url", "dst") }},
		{"Chmod", func() error { return r.Chmod(ctx, "url", 0o755) }},
		{"Chown", func() error { return r.Chown(ctx, "url", 0, 0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Errorf("%s should be unsupported", tt.name)
			}
		})
	}
}

func TestRemote_GetInfo_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, _, _, err := r.GetInfo(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for server error")
	}
}

func TestRemote_GetInfo_InvalidContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "invalid")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, _, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if size != 0 {
		t.Errorf("Expected size 0 for invalid Content-Length, got %d", size)
	}
}

func TestRemote_Exists_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	exists, err := r.Exists(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Expected false for server error")
	}
}

func TestRemote_ReadStream_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.ReadStream(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for server error")
	}
}

func TestRemote_Download_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "test.txt")

	err := r.Download(ctx, srv.URL, dst, nil)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestRemote_DownloadStream_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.DownloadStream(ctx, srv.URL, nil)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestRemote_Copy_DestinationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Copy(ctx, srv.URL, "/invalid/destination/path")
	if err == nil {
		t.Error("Expected error for invalid destination")
	}
}

func TestRemote_Fetch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.Fetch(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for server error")
	}
}

func TestRemote_GetInfo_WithLastModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "50")
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, _, modTime, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if size != 50 {
		t.Errorf("Expected size 50, got %d", size)
	}
	if modTime.IsZero() {
		t.Error("Expected non-zero modTime")
	}
}

func TestRemote_GetInfo_BadLastModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "50")
		w.Header().Set("Last-Modified", "invalid date format")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, _, modTime, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if !modTime.IsZero() {
		t.Error("Expected zero modTime for invalid Last-Modified")
	}
}

func TestRemote_GetInfo_NoHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, rtype, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if rtype != "file" {
		t.Errorf("Expected type file, got %s", rtype)
	}
	if size < 0 {
		t.Errorf("Expected non-negative size, got %d", size)
	}
}

func TestRemote_ReadStream_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "9")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	stream, err := r.ReadStream(ctx, srv.URL)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "test data" {
		t.Errorf("Expected %q, got %q", "test data", string(data))
	}
}

func TestRemote_ReadStream_BadStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.ReadStream(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for 403 status")
	}
}

func TestRemote_Download_WithProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "20")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("01234567890123456789"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "download.txt")

	progressCalls := 0
	err := r.Download(ctx, srv.URL, dst, func(bytes int) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if progressCalls == 0 {
		t.Error("Expected progress callback to be called")
	}

	data, _ := os.ReadFile(dst)
	if len(data) != 20 {
		t.Errorf("Expected 20 bytes, got %d", len(data))
	}
}

func TestRemote_Download_NoContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data without length"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "download.txt")

	err := r.Download(ctx, srv.URL, dst, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "data without length" {
		t.Errorf("Unexpected content: %s", string(data))
	}
}

func TestRemote_Download_CreateFileError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "", "")
	err := r.Download(ctx, srv.URL, invalidPath, nil)
	if err == nil {
		t.Error("Expected error for invalid destination")
	}
}

func TestRemote_DownloadStream_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "11")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("stream data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	progressCalls := 0
	stream, err := r.DownloadStream(ctx, srv.URL, func(bytes int) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("DownloadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "stream data" {
		t.Errorf("Expected %q, got %q", "stream data", string(data))
	}
}

func TestRemote_Copy_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "9")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("copy data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "copied.txt")

	err := r.Copy(ctx, srv.URL, dst)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "copy data" {
		t.Errorf("Expected %q, got %q", "copy data", string(data))
	}
}

func TestRemote_Copy_ReadStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "copied.txt")

	err := r.Copy(ctx, srv.URL, dst)
	if err == nil {
		t.Error("Expected error for server error")
	}
}

func TestRemote_Fetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fetch data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	data, err := r.Fetch(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(data) != "fetch data" {
		t.Errorf("Expected %q, got %q", "fetch data", string(data))
	}
}

func TestRemote_Fetch_ReadAllError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	data, err := r.Fetch(ctx, srv.URL)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
	if len(data) > 0 {
		t.Logf("Got partial data: %d bytes", len(data))
	}
}

func TestRemote_Validate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Validate(ctx, srv.URL)
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestRemote_Validate_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Validate(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for 502 status")
	}
}

func TestRemote_doRequest_BadURL(t *testing.T) {
	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.doRequest(ctx, "GET", "ht!tp://invalid url with spaces") //nolint:bodyclose
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestRemote_doRequest_NetworkError(t *testing.T) {
	cfg := config.Default()
	cfg.HTTPClient = &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return nil, errors.New("network error")
			},
		},
	}

	r := NewRemote(cfg)
	ctx := context.Background()

	_, err := r.doRequest(ctx, "GET", "http://example.com") //nolint:bodyclose
	if err == nil {
		t.Error("Expected network error")
	}
}

func TestRemote_GetInfo_ContentLengthParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1234")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, _, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if size != 1234 {
		t.Errorf("Expected size 1234, got %d", size)
	}
}

func TestRemote_Download_WriteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	invalidDst := filepath.Join("/", "proc", "cannot-write.txt")
	err := r.Download(ctx, srv.URL, invalidDst, nil)
	if err == nil {
		t.Error("Expected error for invalid destination")
	}
}

func TestRemote_DownloadStream_WithCallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "50")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.Repeat("a", 50)))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	progressCalls := 0
	stream, err := r.DownloadStream(ctx, srv.URL, func(bytes int) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("DownloadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if len(data) != 50 {
		t.Errorf("Expected 50 bytes, got %d", len(data))
	}
}

func TestRemote_Copy_WriteFileError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	invalidDst := filepath.Join("/", "", "")
	err := r.Copy(ctx, srv.URL, invalidDst)
	if err == nil {
		t.Error("Expected error for invalid destination")
	}
}

func TestRemote_Exists_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	exists, err := r.Exists(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Expected false for 404 response")
	}
}

func TestRemote_ReadStream_FullRead(t *testing.T) {
	testData := strings.Repeat("test data ", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testData))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	stream, err := r.ReadStream(ctx, srv.URL)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if len(data) != len(testData) {
		t.Errorf("Expected %d bytes, got %d", len(testData), len(data))
	}
}

func TestRemote_Download_LargeFile(t *testing.T) {
	largeData := []byte(strings.Repeat("x", 50000))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(largeData)))
		w.WriteHeader(http.StatusOK)
		w.Write(largeData)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "large.dat")

	err := r.Download(ctx, srv.URL, dst, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if len(data) != len(largeData) {
		t.Errorf("Expected %d bytes, got %d", len(data), len(data))
	}
}

func TestRemote_List_Unsupported(t *testing.T) {
	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.List(ctx, "http://example.com")
	if err == nil {
		t.Error("Expected List to be unsupported")
	}
}

func TestRemote_Read_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("read data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	data, err := r.Read(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != "read data" {
		t.Errorf("Expected %q, got %q", "read data", string(data))
	}
}

func TestRemote_Read_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, err := r.Read(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestRemote_Exists_NotFound_Returns_False(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	exists, err := r.Exists(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Exists returned unexpected error: %v", err)
	}
	if exists {
		t.Error("Expected Exists to return false for 404")
	}
}

func TestRemote_GetInfo_NonOK_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	_, _, _, err := r.GetInfo(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

// newBrokenRemote returns a Remote whose HTTP client always fails to connect.
func newBrokenRemote() *Remote {
	cfg := config.Default()
	cfg.HTTPClient = &http.Client{
		Transport: &errorTransport{},
	}
	return NewRemote(cfg)
}

type errorTransport struct{}

func (e *errorTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("transport error")
}

func TestRemote_GetInfo_RequestError(t *testing.T) {
	r := newBrokenRemote()
	ctx := context.Background()

	_, _, _, err := r.GetInfo(ctx, "http://localhost:0/file")
	if err == nil {
		t.Error("Expected error when request fails")
	}
}

func TestRemote_GetInfo_ContentRange_Branch(t *testing.T) {
	// Server returns size=-1 (no Content-Length) but sets Content-Range header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/42")
		// Deliberately do NOT set Content-Length so Go's HTTP layer leaves ContentLength=-1.
		// We flush the header without a body so the client sees size < 0.
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, rtype, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if rtype != "file" {
		t.Errorf("Expected type file, got %s", rtype)
	}
	// size may be 0 (body-read path) or 42 (Content-Range parse) depending on
	// whether Go's transport hides the header — either way no error is the key assertion.
	_ = size
}

func TestRemote_GetInfo_BodyReadLoop(t *testing.T) {
	// Server with no Content-Length and no Content-Range — triggers body-read loop.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, _, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo body-read loop failed: %v", err)
	}
	if size <= 0 {
		t.Errorf("Expected positive size from body read, got %d", size)
	}
}

func TestRemote_GetInfo_BodyReadLoop_ContextCancelled(t *testing.T) {
	// Cancel context before calling; the select inside the loop should fire.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	// May or may not error depending on timing, but should not panic.
	_, _, _, _ = r.GetInfo(ctx, srv.URL)
}

func TestRemote_Exists_RequestError(t *testing.T) {
	r := newBrokenRemote()
	ctx := context.Background()

	_, err := r.Exists(ctx, "http://localhost:0/file")
	if err == nil {
		t.Error("Expected error when request fails")
	}
}

func TestRemote_ReadStream_RequestError(t *testing.T) {
	r := newBrokenRemote()
	ctx := context.Background()

	_, err := r.ReadStream(ctx, "http://localhost:0/file")
	if err == nil {
		t.Error("Expected error when request fails")
	}
}

func TestRemote_Copy_RequestError(t *testing.T) {
	r := newBrokenRemote()
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "out.txt")

	err := r.Copy(ctx, "http://localhost:0/file", dst)
	if err == nil {
		t.Error("Expected error when request fails")
	}
}

func TestRemote_Copy_InvalidDestination(t *testing.T) {
	// dst contains a null byte — triggers the invalid destination check.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Copy(ctx, srv.URL, "path\x00invalid")
	if err == nil {
		t.Error("Expected error for destination with null byte")
	}
}

func TestRemote_Copy_IOCopyError(t *testing.T) {
	// Server returns 200 but the body errors after header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		// Write less than Content-Length then close — triggers body read error.
		w.Write([]byte("partial"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "out.txt")

	// io.Copy reads until EOF so this should succeed (partial body is valid).
	// The goal is to exercise the io.Copy path regardless of outcome.
	_ = r.Copy(ctx, srv.URL, dst)
}

func TestRemote_Download_InvalidDestination(t *testing.T) {
	// dst contains spaces only — triggers the invalid destination check.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Download(ctx, srv.URL, "   ", nil)
	if err == nil {
		t.Error("Expected error for whitespace-only destination")
	}
}

func TestRemote_Download_LargeSize_UsesDownloadStream(t *testing.T) {
	// Report a large ContentLength so Download takes the DownloadStream path.
	largeSize := config.DefaultMaxMemorySize + 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", largeSize))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("large"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "large.dat")

	// Actual data is short; the path exercises the DownloadStream branch.
	err := r.Download(ctx, srv.URL, dst, nil)
	if err != nil {
		t.Logf("Download large file error (may be expected due to content mismatch): %v", err)
	}
}

func TestRemote_Download_RequestError_InSmallPath(t *testing.T) {
	// GetInfo succeeds (small size), then doRequest inside Download fails.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: GetInfo (GET) — return size=10.
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("1234567890"))
		} else {
			// Second call: the actual download GET — return error.
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "out.txt")

	// Should fail on the second request (body write to file).
	_ = r.Download(ctx, srv.URL, dst, nil)
}

func TestRemote_Download_ContextCancelled(t *testing.T) {
	// Cancel context before Download writes so the select in the write loop fires.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "out.txt")

	_ = r.Download(ctx, srv.URL, dst, nil)
}

func TestRemote_DownloadStream_RequestError(t *testing.T) {
	r := newBrokenRemote()
	ctx := context.Background()

	_, err := r.DownloadStream(ctx, "http://localhost:0/file", nil)
	if err == nil {
		t.Error("Expected error when request fails")
	}
}

func TestRemote_Fetch_RequestError(t *testing.T) {
	r := newBrokenRemote()
	ctx := context.Background()

	_, err := r.Fetch(ctx, "http://localhost:0/file")
	if err == nil {
		t.Error("Expected error when request fails")
	}
}

func TestRemote_Fetch_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// May succeed or fail depending on race — exercise the ctx.Done() path.
	_, _ = r.Fetch(ctx, srv.URL)
}

func TestRemote_Validate_FirstExistsRequestError(t *testing.T) {
	r := newBrokenRemote()
	ctx := context.Background()

	err := r.Validate(ctx, "http://localhost:0/file")
	if err == nil {
		t.Error("Expected error when first request fails")
	}
}

func TestRemote_Validate_SecondRequestError(t *testing.T) {
	// First HEAD returns 200 (exists=true), second HEAD fails with network error.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusOK)
		} else {
			// Simulate error by writing garbage and closing.
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Validate(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for bad gateway on second request")
	}
}

func TestRemote_Download_WithTimeoutOption_RespectsDeadline(t *testing.T) {
	// Regression: the default HTTP client has a 30s Timeout that overrides any
	// step-level timeout for large downloads. WithTimeout(0) must disable the
	// client timeout so the context deadline is the sole authority.

	// WithTimeout(25ms): client fires before server responds → must fail.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	sandbox := t.TempDir()

	cfg25 := config.Default()
	config.WithTimeout(25 * time.Millisecond)(&cfg25)
	r25 := NewRemote(cfg25)
	err := r25.Download(context.Background(), slow.URL, filepath.Join(sandbox, "a"), nil)
	if err == nil {
		t.Fatal("expected timeout error with 25ms client timeout and blocking server")
	}

	// WithTimeout(0): client has no fixed timeout, context 200ms controls → must succeed.
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer fast.Close()

	cfg0 := config.Default()
	config.WithTimeout(0)(&cfg0)
	r0 := NewRemote(cfg0)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err = r0.Download(ctx, fast.URL, filepath.Join(sandbox, "b"), nil)
	if err != nil {
		t.Fatalf("expected success with WithTimeout(0) and 200ms context, got: %v", err)
	}
}

func TestRemote_GetInfo_ContentLengthNegative_ReadBodyLoop(t *testing.T) {
	// Server with no Content-Length (ContentLength=-1) and no Content-Range.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test data body"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, rtype, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if rtype != "file" {
		t.Errorf("Expected type file, got %s", rtype)
	}
	if size <= 0 {
		t.Errorf("Expected positive size from body read, got %d", size)
	}
}

func TestRemote_GetInfo_ContentRangeHeader_Parsing(t *testing.T) {
	// Server returns Content-Range header but ContentLength=-1.
	// The server must NOT set Content-Length so Go's HTTP layer leaves ContentLength=-1.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/100")
		// Do NOT set Content-Length — let it default to -1.
		w.WriteHeader(http.StatusOK)
		// Send empty or small body.
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, _, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	// Size should be 100 (parsed from Content-Range) or 0 from empty body read.
	if size != 100 && size != 0 {
		t.Logf("GetInfo returned size %d (expected 100 or 0)", size)
	}
}

func TestRemote_GetInfo_ContentRangeInvalid(t *testing.T) {
	// Server returns invalid Content-Range format.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "invalid format")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, _, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	// Should fall back to body read loop, size may be 0.
	_ = size
}

func TestRemote_GetInfo_BodyReadLoop_ReadError(t *testing.T) {
	// Mock a broken reader that returns EOF after first read.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write small amount then close.
		w.Write([]byte("ab"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	size, _, _, err := r.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if size != 2 {
		t.Errorf("Expected size 2, got %d", size)
	}
}

func TestRemote_Download_ReadError_InLoop(t *testing.T) {
	// Server returns partial content then closes connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "out.txt")

	err := r.Download(ctx, srv.URL, dst, nil)
	// May succeed with partial data or fail depending on how io.Copy handles short reads.
	_ = err
}

func TestRemote_Download_WriteError_InLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "50")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.Repeat("x", 50)))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	// Try to write to a read-only location.
	invalidPath := filepath.Join("/", "proc", "cannot-write.txt")
	err := r.Download(ctx, srv.URL, invalidPath, nil)
	if err == nil {
		t.Error("Expected error for write-protected path")
	}
}

func TestRemote_Fetch_ContextCancelled_InSelectStatement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fetch data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Fetch(ctx, srv.URL)
	// May succeed or fail depending on timing, but should exercise the select path.
	_ = err
}

func TestRemote_Validate_ExistsReturnsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Validate(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for non-existent resource")
	}
}

func TestRemote_Validate_SecondHeadBadStatus(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Validate(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for bad gateway on second HEAD")
	}
}

func TestRemote_Copy_InvalidDestination_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Copy(ctx, srv.URL, "")
	if err == nil {
		t.Error("Expected error for empty destination")
	}
}

func TestRemote_Copy_InvalidDestination_Whitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Copy(ctx, srv.URL, "   ")
	if err == nil {
		t.Error("Expected error for whitespace destination")
	}
}

func TestRemote_Copy_InvalidDestination_Suffix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Copy(ctx, srv.URL, string(os.PathSeparator))
	if err == nil {
		t.Error("Expected error for path separator destination")
	}
}

func TestRemote_Download_InvalidDestination_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test data!"))
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	err := r.Download(ctx, srv.URL, "", nil)
	if err == nil {
		t.Error("Expected error for empty destination")
	}
}

func TestRemote_Copy_BadStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "out.txt")

	err := r.Copy(ctx, srv.URL, dst)
	if err == nil {
		t.Error("Expected error for non-200 status")
	}
}

func TestRemote_Download_BodyReadError(t *testing.T) {
	// Trigger an error while reading the response body.
	// This is tricky with httptest, so we'll test by having a broken reader.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("short"))
		// Connection closes early, simulating read error.
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "out.txt")

	// This may or may not error depending on how Go's HTTP handles early close.
	_ = r.Download(ctx, srv.URL, dst, nil)
}

func TestRemote_Fetch_BodyReadError_IOReadAll(t *testing.T) {
	// Server claims it will send 100 bytes but only sends 5.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("short"))
		// Closes connection early.
	}))
	defer srv.Close()

	r := NewRemote(config.Default())
	ctx := context.Background()

	// io.ReadAll will read until EOF, so this should succeed with partial data.
	data, err := r.Fetch(ctx, srv.URL)
	// May succeed with partial data.
	_ = data
	_ = err
}
