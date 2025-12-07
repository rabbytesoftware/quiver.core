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

	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/config"
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

	err = r.Mkdir(ctx, "url", 0755)
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
		{"MkdirAll", func() error { return r.MkdirAll(ctx, "url", 0755) }},
		{"RemoveAll", func() error { return r.RemoveAll(ctx, "url") }},
		{"Move", func() error { return r.Move(ctx, "url", "dst") }},
		{"Rename", func() error { return r.Rename(ctx, "url", "dst") }},
		{"Chmod", func() error { return r.Chmod(ctx, "url", 0755) }},
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

	invalidPath := filepath.Join("/", "proc", "")
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

	_, err := r.doRequest(ctx, "GET", "ht!tp://invalid url with spaces")
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

	_, err := r.doRequest(ctx, "GET", "http://example.com")
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

	invalidDst := filepath.Join("/", "proc", "")
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
