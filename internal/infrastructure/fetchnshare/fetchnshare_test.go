package fetchnshare

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestNewFNS(t *testing.T) {
	fns := NewFNS()
	if fns == nil {
		t.Fatal("NewFNS() returned nil")
	}
}

// helper to create a RoundTripper from a function
type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFNS_GetInfo_LocalFileAndDir(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Create temp file
	tf, err := os.CreateTemp("", "getinfo-file-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tf.Name())
	_, _ = tf.WriteString("hello")
	tf.Close()

	info, err := fns.GetInfo(ctx, tf.Name())
	if err != nil {
		t.Fatalf("GetInfo(file) returned error: %v", err)
	}
	if info == nil {
		t.Fatalf("GetInfo(file) returned nil info")
	}
	if info.Size <= 0 {
		t.Errorf("expected positive size for file, got %d", info.Size)
	}

	// Create temp dir
	td, err := os.MkdirTemp("", "getinfo-dir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(td)

	info, err = fns.GetInfo(ctx, td)
	if err != nil {
		t.Fatalf("GetInfo(dir) returned error: %v", err)
	}
	if info == nil {
		t.Fatalf("GetInfo(dir) returned nil info")
	}
	if info.Type != ResourceType("directory") {
		t.Errorf("expected directory type, got %s", info.Type)
	}
}

func TestFNS_GetInfo_RemoteWithHeaders(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Server returns Content-Length and Last-Modified
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "11")
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	info, err := fns.GetInfo(ctx, ts.URL)
	if err != nil {
		t.Fatalf("GetInfo(remote) returned error: %v", err)
	}
	if info == nil {
		t.Fatalf("GetInfo(remote) returned nil info")
	}
	if info.Size != 11 {
		t.Errorf("expected size 11, got %d", info.Size)
	}
	if info.ModTime.IsZero() {
		t.Errorf("expected non-zero ModTime from Last-Modified header")
	}
}

func TestFNS_GetInfo_RemoteNoContentLength_BodyRead(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Server omits Content-Length; GetInfo should read body to compute size
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("small body"))
	}))
	defer ts.Close()

	info, err := fns.GetInfo(ctx, ts.URL)
	if err != nil {
		t.Fatalf("GetInfo(remote no length) returned error: %v", err)
	}
	if info == nil {
		t.Fatalf("GetInfo(remote no length) returned nil info")
	}
	if info.Size <= 0 {
		t.Errorf("expected positive size after reading body, got %d", info.Size)
	}
}

func TestFNS_GetInfo_CreateRequestError(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Pass an invalid URL that causes http.NewRequestWithContext to fail
	_, err := fns.GetInfo(ctx, "http://%")
	if err == nil {
		t.Errorf("expected error when creating request, got nil")
	}
}

func TestFNS_GetInfo_HTTPClientDoError(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Replace the default client with one whose Transport returns an error
	origClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("simulated transport error")
		}),
	}
	defer func() { http.DefaultClient = origClient }()

	_, err := fns.GetInfo(ctx, "http://example.invalid")
	if err == nil {
		t.Errorf("expected error when HTTP client Do fails, got nil")
	}
}

func TestFNS_Exists_LocalFileAndDir(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()
	// Create temp file
	tf, err := os.CreateTemp("", "exists-file-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tf.Name())

	exists, err := fns.Exists(ctx, tf.Name())
	if err != nil {
		t.Fatalf("Exists(file) returned error: %v", err)
	}
	if !exists {
		t.Errorf("Exists(file) should return true for existing file")
	}
	// Create temp dir
	td, err := os.MkdirTemp("", "exists-dir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(td)

	exists, err = fns.Exists(ctx, td)
	if err != nil {
		t.Fatalf("Exists(dir) returned error: %v", err)
	}
	if !exists {
		t.Errorf("Exists(dir) should return true for existing dir")
	}
}

func TestFNS_Exists_CreateRequestError(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Pass an invalid URL that causes http.NewRequestWithContext to fail
	exists, err := fns.Exists(ctx, "http://%")
	if err == nil {
		t.Errorf("expected error when creating request, got nil")
	}
	if exists {
		t.Errorf("expected exists to be false on error, got true")
	}
}

func TestFNS_Exists_HTTPClientDoError(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Replace the default client with one whose Transport returns an error
	origClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("simulated transport error")
		}),
	}
	defer func() { http.DefaultClient = origClient }()

	exists, err := fns.Exists(ctx, "http://example.invalid")
	if err == nil {
		t.Errorf("expected error when creating request, got nil")
	}
	if exists {
		t.Errorf("expected exists to be false on error, got true")
	}
}

func TestFNS_IsDir(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	isDir, err := fns.IsDir(ctx, "test-path")
	if err != nil {
		t.Errorf("IsDir() returned error: %v", err)
	}
	if isDir {
		t.Error("IsDir() should return false for unimplemented method")
	}
}

func TestFNS_IsFile(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	isFile, err := fns.IsFile(ctx, "test-path")
	if err != nil {
		t.Errorf("IsFile() returned error: %v", err)
	}
	if isFile {
		t.Error("IsFile() should return false for unimplemented method")
	}
}

func TestFNS_Read(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	data, err := fns.Read(ctx, "test-path")
	if err != nil {
		t.Errorf("Read() returned error: %v", err)
	}
	if data != nil {
		t.Error("Read() should return nil for unimplemented method")
	}
}

func TestFNS_ReadStream(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	reader, err := fns.ReadStream(ctx, "test-path")
	if err != nil {
		t.Errorf("ReadStream() returned error: %v", err)
	}
	if reader != nil {
		t.Error("ReadStream() should return nil for unimplemented method")
	}
}

func TestFNS_Write(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Write(ctx, "test-path", []byte("test data"))
	if err != nil {
		t.Errorf("Write() returned error: %v", err)
	}
}

func TestFNS_WriteStream(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.WriteStream(ctx, "test-path", nil)
	if err != nil {
		t.Errorf("WriteStream() returned error: %v", err)
	}
}

func TestFNS_Append(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Append(ctx, "test-path", []byte("test data"))
	if err != nil {
		t.Errorf("Append() returned error: %v", err)
	}
}

func TestFNS_List(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	list, err := fns.List(ctx, "test-path")
	if err != nil {
		t.Errorf("List() returned error: %v", err)
	}
	if list != nil {
		t.Error("List() should return nil for unimplemented method")
	}
}

func TestFNS_Mkdir(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Mkdir(ctx, "test-path", 0755)
	if err != nil {
		t.Errorf("Mkdir() returned error: %v", err)
	}
}

func TestFNS_MkdirAll(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.MkdirAll(ctx, "test-path", 0755)
	if err != nil {
		t.Errorf("MkdirAll() returned error: %v", err)
	}
}

func TestFNS_Remove(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Remove(ctx, "test-path")
	if err != nil {
		t.Errorf("Remove() returned error: %v", err)
	}
}

func TestFNS_RemoveAll(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.RemoveAll(ctx, "test-path")
	if err != nil {
		t.Errorf("RemoveAll() returned error: %v", err)
	}
}

func TestFNS_Copy(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Copy(ctx, "src", "dst")
	if err != nil {
		t.Errorf("Copy() returned error: %v", err)
	}
}

func TestFNS_Move(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Move(ctx, "src", "dst")
	if err != nil {
		t.Errorf("Move() returned error: %v", err)
	}
}

func TestFNS_Rename(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Rename(ctx, "src", "dst")
	if err != nil {
		t.Errorf("Rename() returned error: %v", err)
	}
}

func TestFNS_Chmod(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Chmod(ctx, "test-path", 0755)
	if err != nil {
		t.Errorf("Chmod() returned error: %v", err)
	}
}

func TestFNS_Chown(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Chown(ctx, "test-path", 1000, 1000)
	if err != nil {
		t.Errorf("Chown() returned error: %v", err)
	}
}

func TestFNS_Download(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	progress := func(bytes int) {}
	err := fns.Download(ctx, "http://example.com", "dst", progress)
	if err != nil {
		t.Errorf("Download() returned error: %v", err)
	}
}

func TestFNS_DownloadStream(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	progress := func(bytes int) {}
	reader, err := fns.DownloadStream(ctx, "http://example.com", progress)
	if err != nil {
		t.Errorf("DownloadStream() returned error: %v", err)
	}
	if reader != nil {
		t.Error("DownloadStream() should return nil for unimplemented method")
	}
}

func TestFNS_Fetch(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	data, err := fns.Fetch(ctx, "http://example.com")
	if err != nil {
		t.Errorf("Fetch() returned error: %v", err)
	}
	if data != nil {
		t.Error("Fetch() should return nil for unimplemented method")
	}
}

func TestFNS_CacheGet(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	data, err := fns.CacheGet(ctx, "test-key")
	if err != nil {
		t.Errorf("CacheGet() returned error: %v", err)
	}
	if data != nil {
		t.Error("CacheGet() should return nil for unimplemented method")
	}
}

func TestFNS_CacheSet(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.CacheSet(ctx, "test-key", []byte("test data"), time.Hour)
	if err != nil {
		t.Errorf("CacheSet() returned error: %v", err)
	}
}

func TestFNS_CacheDelete(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.CacheDelete(ctx, "test-key")
	if err != nil {
		t.Errorf("CacheDelete() returned error: %v", err)
	}
}

func TestFNS_CacheClear(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.CacheClear(ctx)
	if err != nil {
		t.Errorf("CacheClear() returned error: %v", err)
	}
}

func TestFNS_Resolve(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, resourceType, err := fns.Resolve(ctx, "test-path")
	if err != nil {
		t.Errorf("Resolve() returned error: %v", err)
	}
	if path != "" {
		t.Error("Resolve() should return empty string for unimplemented method")
	}
	if resourceType != "" {
		t.Error("Resolve() should return empty string for unimplemented method")
	}
}

func TestFNS_Validate(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Validate(ctx, "test-path")
	if err != nil {
		t.Errorf("Validate() returned error: %v", err)
	}
}

func TestFNS_TempFile(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempFile(ctx, "test-*")
	if err != nil {
		t.Errorf("TempFile() returned error: %v", err)
	}
	if path != "" {
		t.Error("TempFile() should return empty string for unimplemented method")
	}
}

func TestFNS_TempDir(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempDir(ctx, "test-*")
	if err != nil {
		t.Errorf("TempDir() returned error: %v", err)
	}
	if path != "" {
		t.Error("TempDir() should return empty string for unimplemented method")
	}
}

func TestFNS_Walk(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	walkFn := func(path string, info ResourceInfo, err error) error {
		return nil
	}

	err := fns.Walk(ctx, "test-root", walkFn)
	if err != nil {
		t.Errorf("Walk() returned error: %v", err)
	}
}
