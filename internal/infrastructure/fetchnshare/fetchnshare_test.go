package fetchnshare

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/config"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/strategies"
)

func TestNewFNS(t *testing.T) {
	fns := NewFNS()
	if fns == nil {
		t.Fatal("NewFNS() returned nil")
	}
}

func TestFNS_ContextCancellation(t *testing.T) {
	t.Run("WriteStream with cancelled context", func(t *testing.T) {
		fns := NewFNS()
		sandbox := t.TempDir()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		testFile := filepath.Join(sandbox, "test.txt")
		reader := io.NopCloser(strings.NewReader("test data"))

		err := fns.WriteStream(ctx, testFile, reader)
		if err == nil {
			t.Error("Expected error with cancelled context")
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Got error (acceptable): %v", err)
		}
	})

	t.Run("Fetch with timeout", func(t *testing.T) {
		fns := NewFNS()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Write([]byte("data"))
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond)

		_, err := fns.Fetch(ctx, srv.URL)
		if err == nil {
			t.Error("Expected timeout error")
		}
	})
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

	// Test file case
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

	// Test directory case
	info, err = fns.GetInfo(ctx, td)
	if err != nil {
		t.Fatalf("GetInfo(dir) returned error: %v", err)
	}
	if info == nil {
		t.Fatalf("GetInfo(dir) returned nil info")
	}
	if info.Type != ResourceType("dir") {
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

func TestFNS_Exists_LocalFileAndDir(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Create temp file
	tf, err := os.CreateTemp("", "exists-file-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tf.Name())

	// Test file case
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

	// Test directory case
	exists, err = fns.Exists(ctx, td)
	if err != nil {
		t.Fatalf("Exists(dir) returned error: %v", err)
	}
	if !exists {
		t.Errorf("Exists(dir) should return true for existing dir")
	}
}

func TestFNS_Exists_Remote(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Server returns 200 OK
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	exists, err := fns.Exists(ctx, ts.URL)
	if err != nil {
		t.Errorf("Exists(remote) returned error: %v", err)
	}
	if !exists {
		t.Errorf("Exists(remote) should return true for 200 OK")
	}
}

func TestFNS_IsDir_IsFile(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Create temp dir
	td, err := os.MkdirTemp("", "dir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(td)

	// Create temp file
	tf, err := os.CreateTemp("", "dir-file-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tf.Name())

	// IsDir on Dir
	isDir, err := fns.IsDir(ctx, td)
	if err != nil {
		t.Errorf("IsDir() returned error: %v", err)
	}
	if !isDir {
		t.Error("IsDir() should return true for existing directory")
	}

	// IsFile on Dir
	isFile, err := fns.IsFile(ctx, td)
	if err != nil {
		t.Errorf("isFile() returned error: %v", err)
	}
	if isFile {
		t.Error("isFile() should return false for directory")
	}

	// IsDir on File
	isDir, err = fns.IsDir(ctx, tf.Name())
	if err != nil {
		t.Errorf("IsDir() returned error: %v", err)
	}
	if isDir {
		t.Error("IsDir() should return false for file")
	}

	// IsFile on File
	isFile, err = fns.IsFile(ctx, tf.Name())
	if err != nil {
		t.Errorf("isFile() returned error: %v", err)
	}
	if !isFile {
		t.Error("isFile() should return true for file")
	}
}

func TestFNS_Read_SmallFile(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()
	data := []byte("hello")

	// Create temp file
	tf, err := os.CreateTemp("", "exists-file-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tf.Name())

	// Write stuff into it
	if _, err := tf.Write(data); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tf.Close()

	got, stream, err := fns.Read(ctx, tf.Name())
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if stream != nil {
		t.Error("Read() should return nil ReadCloser for small files")
	}
	if string(got) != "hello" {
		t.Errorf("Read() returned incorrect data: got %q, want %q", got, data)
	}
}

func TestFNS_Read_LargeFile(t *testing.T) {
	ctx := context.Background()
	f := NewFNS()

	largeData := make([]byte, config.DefaultMaxMemorySize+1)

	// Create temp file
	tf, err := os.CreateTemp("", "large-file-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tf.Name())
	defer tf.Close()

	// Write stuff into it
	if _, err := tf.Write(largeData); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, rc, err := f.Read(ctx, tf.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil data for large file")
	}
	if rc == nil {
		t.Fatalf("expected ReadCloser for large file")
	}
	if err = rc.Close(); err != nil {
		t.Fatalf("stream should NOT be closed for large file")
	}
	rc.Close()
}

func TestFNS_Read_IsDirectory(t *testing.T) {
	f := NewFNS()

	// Create temp dir
	td, err := os.MkdirTemp("", "dir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(td)

	_, _, err = f.Read(context.Background(), td)
	if err == nil {
		t.Fatalf("expected error for directory")
	}
}

func TestFNS_ReadStream_Remote(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Server returns large body
	data := []byte("remember this.")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer ts.Close()

	reader, err := fns.ReadStream(ctx, ts.URL)
	if err != nil {
		t.Errorf("ReadStream(remote) returned error: %v", err)
	}
	if reader == nil {
		t.Error("ReadStream(remote) should return non-nil ReadCloser")
	}
	defer reader.Close()

	// Read data from reader and verify size
	n, err := io.ReadAll(reader)
	if err != nil && err.Error() != "EOF" {
		t.Errorf("error reading from ReadCloser: %v", err)
	}
	if len(n) != len(data) {
		t.Errorf("ReadStream(remote) returned incorrect size: got %d, want %d", len(n), config.DefaultMaxMemorySize+1)
	}
}

func TestFNS_ReadStream_Local(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	tflarge, err := os.CreateTemp("", "temp-large-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tflarge.Name())
	largeData := make([]byte, config.DefaultMaxMemorySize+1) // larger than Maxsize

	_, err = tflarge.Write(largeData)
	tflarge.Close()

	if err != nil {
		t.Fatalf("failed to write to large temp file: %v", err)
	}

	reader, err := fns.ReadStream(ctx, tflarge.Name())

	if err != nil {
		t.Errorf("ReadStream() returned error: %v", err)
	}
	if reader == nil {
		t.Error("ReadStream() should return non-nil ReadCloser")
	}
	defer reader.Close()

	// Read data from reader and verify size
	n, err := io.ReadAll(reader)
	if err != nil && err.Error() != "EOF" {
		t.Errorf("error reading from ReadCloser: %v", err)
	}
	if len(n) != config.DefaultMaxMemorySize+1 {
		t.Errorf("ReadStream() returned incorrect size: got %d, want %d", len(n), config.DefaultMaxMemorySize+1)
	}
}
func TestFNS_WriteAndWriteStream(t *testing.T) {
	f := NewFNS()
	ctx := context.Background()
	data := make([]byte, config.DefaultMaxMemorySize+1) // larger than DefaultMaxMemorySize so that Write() calls WriteStream()

	// Test remote
	err := f.Write(ctx, "http://example.com/resource", []byte("test data"))
	if err == nil {
		t.Error("Write(remote) should return error for unimplemented method")
	}

	// tempFile
	tf, err := os.CreateTemp("", "write-local-*.txt")
	defer os.Remove(tf.Name())
	tf.Close()

	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Test local (small data)
	err = f.Write(ctx, tf.Name(), []byte("test data"))
	if err != nil {
		t.Errorf("Write() with small data returned error: %v", err)
	}

	// Test WriteStream()
	// Test local (large data)
	err = f.Write(ctx, tf.Name(), data)
	if err != nil {
		t.Errorf("Write() with large data returned error: %v", err)
	}
	content, err := os.ReadFile(tf.Name())
	if err != nil {
		t.Fatalf("error reading file %v", err)
	}
	if len(content) != len(data) {
		t.Errorf("wrong content written, expected length %d, got %d", len(data), len(content))
	}
}

func TestFNS_Append_NewAndExisting(t *testing.T) {
	f := NewFNS()
	ctx := context.Background()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "file.txt")

	// Test remote
	err := f.Append(ctx, "https://", []byte("beep"))
	if err == nil {
		t.Errorf("Append() should return error for unimplemented method, got %v", err)

	}

	// Test new file
	err = f.Append(context.Background(), path, []byte("Hello "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check file exists (first time)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading file: %v", err)
	}

	// Check content (first time)
	if string(content) != "Hello " {
		t.Fatalf("expected %q, got %q", "Hello ", string(content))
	}

	// Test existing file
	err = f.Append(ctx, path, []byte("world."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check file exists (second time)
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading file: %v", err)
	}

	// Check content (second time)
	if string(content) != "Hello world." {
		t.Fatalf("expected %q, got %q", "Hello world.", string(content))
	}
}

func TestFNS_Append_CreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "a", "b", "c", "file.txt")

	l := strategies.NewLocal(config.Default())

	err := l.Append(context.Background(), nested, []byte("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(nested)
	if err != nil {
		t.Fatalf("failed reading file: %v", err)
	}

	if string(content) != "data" {
		t.Fatalf("expected \"data\", got %q", string(content))
	}
}

func TestFNS_List(t *testing.T) {
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	testFile := filepath.Join(sandbox, "file1.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	list, err := fns.List(ctx, sandbox)
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 item, got %d", len(list))
	}
	if len(list) > 0 && list[0].Path != testFile {
		t.Errorf("expected path %s, got %s", testFile, list[0].Path)
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
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	testDir := filepath.Join(sandbox, "test-dir")
	os.Mkdir(testDir, 0755)

	err := fns.RemoveAll(ctx, testDir)
	if err != nil {
		t.Errorf("RemoveAll() returned error: %v", err)
	}
}

func TestFNS_Copy(t *testing.T) {
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	err := fns.Copy(ctx, src, dst)
	if err != nil {
		t.Errorf("Copy() returned error: %v", err)
	}
}

func TestFNS_Move(t *testing.T) {
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	err := fns.Move(ctx, src, dst)
	if err != nil {
		t.Errorf("Move() returned error: %v", err)
	}
}

func TestFNS_Rename(t *testing.T) {
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	err := fns.Rename(ctx, src, dst)
	if err != nil {
		t.Errorf("Rename() returned error: %v", err)
	}
}

func TestFNS_Chmod(t *testing.T) {
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := fns.Chmod(ctx, testFile, 0755)
	if err != nil {
		t.Errorf("Chmod() returned error: %v", err)
	}
}

func TestFNS_Chown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chown not supported on Windows")
	}
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := fns.Chown(ctx, testFile, os.Getuid(), os.Getgid())
	if err != nil {
		t.Errorf("Chown() returned error: %v", err)
	}
}

func TestFNS_Download(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()
	dst := filepath.Join(t.TempDir(), "downloaded_file")
	defer os.Remove(dst)

	progress := func(bytes int) {}
	err := fns.Download(ctx, "http://example.com", dst, progress)
	if err != nil {
		t.Errorf("Download() returned error: %v", err)
	}
}

func TestFNS_DownloadStream(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test data"))
	}))
	defer srv.Close()

	progress := func(bytes int) {}
	reader, err := fns.DownloadStream(ctx, srv.URL, progress)
	if err != nil {
		t.Errorf("DownloadStream() returned error: %v", err)
	}
	if reader == nil {
		t.Error("DownloadStream() should return reader")
	}
	if reader != nil {
		reader.Close()
	}
}

func TestFNS_Fetch(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test data"))
	}))
	defer srv.Close()

	data, err := fns.Fetch(ctx, srv.URL)
	if err != nil {
		t.Errorf("Fetch() returned error: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("Fetch() returned %q, want %q", string(data), "test data")
	}
}

func TestFNS_Resolve(t *testing.T) {
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	path, resourceType, err := fns.Resolve(ctx, testFile)
	if err != nil {
		t.Errorf("Resolve() returned error: %v", err)
	}
	if path == "" {
		t.Error("Resolve() should return non-empty path")
	}
	if resourceType != "file" {
		t.Errorf("Resolve() returned type %q, want %q", resourceType, "file")
	}
}

func TestFNS_Validate(t *testing.T) {
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := fns.Validate(ctx, testFile)
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
	if path == "" {
		t.Error("TempFile() should return non-empty path")
	}
	defer os.Remove(path)
}

func TestFNS_TempDir(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempDir(ctx, "test-*")
	if err != nil {
		t.Errorf("TempDir() returned error: %v", err)
	}
	if path == "" {
		t.Error("TempDir() should return non-empty path")
	}
	defer os.RemoveAll(path)
}

func TestFNS_Walk(t *testing.T) {
	sandbox := t.TempDir()
	fns := NewFNS()
	ctx := context.Background()

	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	var count int
	walkFn := func(path string, info ResourceInfo, err error) error {
		count++
		return nil
	}

	err := fns.Walk(ctx, sandbox, walkFn)
	if err != nil {
		t.Errorf("Walk() returned error: %v", err)
	}
	if count == 0 {
		t.Error("Walk() should have visited at least one path")
	}
}

func TestNewFNS_WithOptions(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}

	fns := NewFNS(
		config.WithHTTPClient(customClient),
		config.WithMaxMemorySize(2*1024*1024),
		config.WithBufferSize(16*1024),
	)

	if fns == nil {
		t.Fatal("NewFNS() returned nil")
	}
}

func TestFNS_GetInfo_RemoteURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fns := NewFNS()
	ctx := context.Background()

	info, err := fns.GetInfo(ctx, srv.URL)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if info.Size != 100 {
		t.Errorf("Expected size 100, got %d", info.Size)
	}
}

func TestFNS_Read_EmptyPath(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	_, _, err := fns.Read(ctx, "")
	if err == nil {
		t.Error("Expected error for empty path")
	}
}

func TestFNS_Read_Directory(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	ctx := context.Background()

	_, _, err := fns.Read(ctx, sandbox)
	if err == nil {
		t.Error("Expected error when reading directory")
	}
}

func TestFNS_List_NotDirectory(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	_, err := fns.List(ctx, testFile)
	if err == nil {
		t.Error("Expected error when listing file")
	}
}

func TestFNS_Copy_SourceNotExist(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "dst.txt")

	ctx := context.Background()

	err := fns.Copy(ctx, "/nonexistent/source", dst)
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestFNS_Chown_NotFound(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Chown(ctx, "/nonexistent/path", 0, 0)
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestFNS_Download_InvalidURL(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "test.txt")

	ctx := context.Background()

	err := fns.Download(ctx, "http://invalid.nonexistent.url", dst, nil)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestFNS_Resolve_LocalFile(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	path, resourceType, err := fns.Resolve(ctx, testFile)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if path == "" {
		t.Error("Expected non-empty path")
	}
	if resourceType != "file" {
		t.Errorf("Expected type file, got %s", resourceType)
	}
}

func TestFNS_Resolve_Directory(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()

	ctx := context.Background()

	path, resourceType, err := fns.Resolve(ctx, sandbox)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if path == "" {
		t.Error("Expected non-empty path")
	}
	if resourceType != "dir" {
		t.Errorf("Expected type dir, got %s", resourceType)
	}
}

func TestFNS_Validate_NotFound(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Validate(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestFNS_TempFile_WithPattern(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempFile(ctx, "test-*.txt")
	if err != nil {
		t.Fatalf("TempFile failed: %v", err)
	}
	defer os.Remove(path)

	if !strings.Contains(path, "test-") || !strings.HasSuffix(path, ".txt") {
		t.Errorf("TempFile pattern not applied: %s", path)
	}
}

func TestFNS_TempDir_WithPattern(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempDir(ctx, "test-*")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(path)

	if !strings.Contains(path, "test-") {
		t.Errorf("TempDir pattern not applied: %s", path)
	}
}

func TestFNS_Walk_EmptyDirectory(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	ctx := context.Background()

	count := 0
	walkFn := func(path string, info ResourceInfo, err error) error {
		count++
		return nil
	}

	err := fns.Walk(ctx, sandbox, walkFn)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	if count < 1 {
		t.Errorf("Expected at least 1 path, got %d", count)
	}
}

func TestFNS_Walk_ErrorInCallback(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	os.WriteFile(filepath.Join(sandbox, "test.txt"), []byte("test"), 0644)

	ctx := context.Background()

	expectedErr := errors.New("callback error")
	walkFn := func(path string, info ResourceInfo, err error) error {
		return expectedErr
	}

	err := fns.Walk(ctx, sandbox, walkFn)
	if err != expectedErr {
		t.Errorf("Expected callback error to be returned, got: %v", err)
	}
}

func TestFNS_Walk_WithNestedDirectories(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()

	os.MkdirAll(filepath.Join(sandbox, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(sandbox, "a", "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(sandbox, "a", "b", "file2.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(sandbox, "a", "b", "c", "file3.txt"), []byte("test"), 0644)

	ctx := context.Background()

	count := 0
	walkFn := func(path string, info ResourceInfo, err error) error {
		count++
		return nil
	}

	err := fns.Walk(ctx, sandbox, walkFn)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	if count < 7 {
		t.Errorf("Expected at least 7 paths (1 root + 3 dirs + 3 files), got %d", count)
	}
}

func TestFNS_Walk_NonexistentPath(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	err := fns.Walk(ctx, "/nonexistent/path", func(path string, info ResourceInfo, err error) error {
		return nil
	})
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestFNS_Download_WithProgressCallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "15")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("download test!!"))
	}))
	defer srv.Close()

	fns := NewFNS()
	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "downloaded.txt")

	ctx := context.Background()

	progressCalls := 0
	err := fns.Download(ctx, srv.URL, dst, func(bytes int) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if progressCalls == 0 {
		t.Error("Expected progress callback to be called")
	}

	data, _ := os.ReadFile(dst)
	if len(data) != 15 {
		t.Errorf("Expected 15 bytes, got %d", len(data))
	}
}

func TestFNS_Resolve_RemoteURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fns := NewFNS()
	ctx := context.Background()

	path, resourceType, err := fns.Resolve(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if path != srv.URL {
		t.Errorf("Expected path %s, got %s", srv.URL, path)
	}
	if resourceType != "file" {
		t.Errorf("Expected type file, got %s", resourceType)
	}
}

func TestFNS_Resolve_InvalidPath(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	_, _, err := fns.Resolve(ctx, "/nonexistent/path/file.txt")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestFNS_List_WithMultipleFiles(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()

	os.WriteFile(filepath.Join(sandbox, "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(sandbox, "file2.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(sandbox, "file3.txt"), []byte("test"), 0644)
	os.Mkdir(filepath.Join(sandbox, "subdir"), 0755)

	ctx := context.Background()

	names, err := fns.List(ctx, sandbox)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 4 {
		t.Errorf("Expected 4 items, got %d", len(names))
	}
}

func TestFNS_Copy_RemoteToLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "11")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("remote copy"))
	}))
	defer srv.Close()

	fns := NewFNS()
	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "copied.txt")

	ctx := context.Background()

	err := fns.Copy(ctx, srv.URL, dst)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "remote copy" {
		t.Errorf("Expected %q, got %q", "remote copy", string(data))
	}
}

func TestFNS_Chown_Success(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	err := fns.Chown(ctx, testFile, os.Getuid(), os.Getgid())
	if err != nil && !strings.Contains(err.Error(), "Windows") {
		t.Errorf("Chown failed: %v", err)
	}
}

func TestFNS_TempFile_DefaultPattern(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempFile(ctx, "")
	if err != nil {
		t.Fatalf("TempFile failed: %v", err)
	}
	defer os.Remove(path)

	if path == "" {
		t.Error("Expected non-empty path")
	}

	_, err = os.Stat(path)
	if err != nil {
		t.Errorf("TempFile should exist: %v", err)
	}
}

func TestFNS_TempDir_DefaultPattern(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempDir(ctx, "")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(path)

	if path == "" {
		t.Error("Expected non-empty path")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("TempDir should exist: %v", err)
	}
	if info != nil && !info.IsDir() {
		t.Error("Expected path to be directory")
	}
}

func TestFNS_Validate_RemoteURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fns := NewFNS()
	ctx := context.Background()

	err := fns.Validate(ctx, srv.URL)
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestFNS_Validate_LocalFile(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	err := fns.Validate(ctx, testFile)
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestFNS_GetInfo_Error(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	_, err := fns.GetInfo(ctx, "/nonexistent/path/file.txt")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestFNS_Read_GetInfoError(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	_, _, err := fns.Read(ctx, "/nonexistent/path/file.txt")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestFNS_Copy_LocalToLocal(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()

	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("copy test"), 0644)

	ctx := context.Background()

	err := fns.Copy(ctx, src, dst)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "copy test" {
		t.Errorf("Expected %q, got %q", "copy test", string(data))
	}
}

func TestFNS_Download_BadURL(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "test.txt")

	ctx := context.Background()

	err := fns.Download(ctx, "http://definitely-not-exist-123456789.invalid", dst, nil)
	if err == nil {
		t.Error("Expected error for bad URL")
	}
}

func TestFNS_Validate_RemoteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fns := NewFNS()
	ctx := context.Background()

	err := fns.Validate(ctx, srv.URL)
	if err == nil {
		t.Error("Expected error for 500 status")
	}
}

func TestFNS_Walk_WithError(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()

	restrictedDir := filepath.Join(sandbox, "restricted")
	os.Mkdir(restrictedDir, 0755)
	os.WriteFile(filepath.Join(restrictedDir, "file.txt"), []byte("test"), 0644)

	os.Chmod(restrictedDir, 0000)
	defer os.Chmod(restrictedDir, 0755)

	ctx := context.Background()

	visitCount := 0
	walkFn := func(path string, info ResourceInfo, err error) error {
		visitCount++
		if err != nil {
			return nil
		}
		return nil
	}

	fns.Walk(ctx, sandbox, walkFn)

	if visitCount < 1 {
		t.Error("Expected at least one path to be visited")
	}
}

func TestFNS_Walk_SkipError(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()

	os.MkdirAll(filepath.Join(sandbox, "a"), 0755)
	os.WriteFile(filepath.Join(sandbox, "a", "file.txt"), []byte("test"), 0644)

	ctx := context.Background()

	paths := []string{}
	walkFn := func(path string, info ResourceInfo, err error) error {
		paths = append(paths, path)
		return nil
	}

	err := fns.Walk(ctx, sandbox, walkFn)
	if err != nil {
		t.Errorf("Walk failed: %v", err)
	}
	if len(paths) < 2 {
		t.Errorf("Expected at least 2 paths, got %d", len(paths))
	}
}

func TestFNS_List_EmptyDirectory(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()

	ctx := context.Background()

	names, err := fns.List(ctx, sandbox)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("Expected 0 items in empty directory, got %d", len(names))
	}
}

func TestFNS_Resolve_HTTPUrl(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fns := NewFNS()
	ctx := context.Background()

	path, resourceType, err := fns.Resolve(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if path != srv.URL {
		t.Errorf("Expected path %s, got %s", srv.URL, path)
	}
	if resourceType != "file" {
		t.Errorf("Expected type file, got %s", resourceType)
	}
}

func TestFNS_TempFile_ErrorCreation(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempFile(ctx, "test-*")
	if err != nil {
		t.Logf("TempFile creation issue: %v", err)
	}
	if path != "" {
		defer os.Remove(path)
	}
}

func TestFNS_TempDir_ErrorCreation(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	path, err := fns.TempDir(ctx, "test-*")
	if err != nil {
		t.Logf("TempDir creation issue: %v", err)
	}

	if path != "" {
		defer os.RemoveAll(path)
	}
}

func TestFNS_Chown_LocalPath(t *testing.T) {
	fns := NewFNS()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	err := fns.Chown(ctx, testFile, os.Getuid(), os.Getgid())
	if err != nil && runtime.GOOS != "windows" && os.Getuid() == 0 {
		t.Errorf("Chown failed: %v", err)
	}
}
