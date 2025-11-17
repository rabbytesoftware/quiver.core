package fetchnshare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestNewFNS(t *testing.T) {
	fns := NewFNS()
	if fns == nil {
		t.Fatal("NewFNS() returned nil")
	}
}

func TestFNS_ContextCancellation(t *testing.T) {
	sandbox := t.TempDir()

	// Force it to work on another directory: the sandbox
	// This way, even if it panics or an error is found, no extra file like "some-path" will be created
	oldwd, _ := os.Getwd()
	os.Chdir(sandbox)
	t.Cleanup(func() { os.Chdir(oldwd) })

	// Capture the initial clean snapshot
	baseline := snapshot(t, sandbox)

	// Dummy ReadCloser for WriteStream
	rc := io.NopCloser(strings.NewReader("test"))

	// Canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	<-ctx.Done()

	// Instantiate FNS interface
	var s ResourceStrategy = NewFNS()

	// List of method names and their arguments
	tests := []struct {
		name string
		args []any
	}{
		{"GetInfo", []any{ctx, "some-path"}},
		{"Exists", []any{ctx, "some-path"}},
		{"IsDir", []any{ctx, "some-path"}},
		{"IsFile", []any{ctx, "some-path"}},
		{"Read", []any{ctx, "some-path"}},
		{"ReadStream", []any{ctx, "some-path"}},
		{"Write", []any{ctx, "some-path", []byte("data")}},
		{"WriteStream", []any{ctx, "some-path", rc}},
		{"Append", []any{ctx, "some-path", []byte("data")}},
		{"List", []any{ctx, "some-path"}},
		{"Mkdir", []any{ctx, "some-path", os.FileMode(0755)}},
		{"MkdirAll", []any{ctx, "some-path", os.FileMode(0755)}},
		{"Remove", []any{ctx, "some-path"}},
		{"RemoveAll", []any{ctx, "some-path"}},
		{"Copy", []any{ctx, "src", "dst"}},
		{"Move", []any{ctx, "src", "dst"}},
		{"Rename", []any{ctx, "src", "dst"}},
		{"Chmod", []any{ctx, "some-path", os.FileMode(0755)}},
		{"Chown", []any{ctx, "some-path", 1000, 1000}},
		{"Download", []any{ctx, "http://example.com", "dst", func(int) {}}},
		{"DownloadStream", []any{ctx, "http://example.com", func(int) {}}},
		{"Fetch", []any{ctx, "http://example.com"}},
		{"CacheGet", []any{ctx, "test-key"}},
		{"CacheSet", []any{ctx, "test-key", []byte("data"), time.Hour}},
		{"CacheDelete", []any{ctx, "test-key"}},
		{"CacheClear", []any{ctx}},
		{"Resolve", []any{ctx, "some-path"}},
		{"Validate", []any{ctx, "some-path"}},
		{"TempFile", []any{ctx, "test-*"}},
		{"TempDir", []any{ctx, "test-*"}},
		{"Walk", []any{ctx, "test-root", func(string, ResourceInfo, error) error { return nil }}},
	}

	sv := reflect.ValueOf(s)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Make sure to clean the sandbox at the end
			defer resetSandbox(t, sandbox, baseline)

			method := sv.MethodByName(tt.name)
			if !method.IsValid() {
				t.Fatalf("method %s not found", tt.name)
			}

			// Convert args
			in := make([]reflect.Value, len(tt.args))
			for i, arg := range tt.args {
				in[i] = reflect.ValueOf(arg)
			}

			// Recover from panics so cleanup still runs
			defer func() {
				if r := recover(); r != nil {
					t.Logf("%s panicked (expected if not context-safe): %v", tt.name, r)
				}
			}()

			out := method.Call(in)

			// Last return must be error
			errVal := out[len(out)-1].Interface()

			// Check whether it's ctx.Err(), fail if not.
			if errVal != nil {
				if !errors.Is(errVal.(error), context.Canceled) {
					t.Fatalf("%s expected ctx.Err() error got: %v", tt.name, errVal)
				}
			} else {
				t.Fatalf("%s expected ctx.Err() error got <nil>", tt.name)
			}
		})
	}
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

	// mock readCloser
	rc := &mockRC{Reader: bytes.NewReader([]byte("ignored"))}

	// mockStrat makes sure that, when ran, Read() will assume the file is too large and attempt to call ReadStream()
	mockStrat := &mockStrategy{
		ReadStreamFunc: func(ctx context.Context, path string) (io.ReadCloser, error) {
			return rc, nil
		},
		GetInfoFunc: func(ctx context.Context, path string) (*ResourceInfo, error) {
			return &ResourceInfo{Size: Maxsize + 1, Type: "file"}, nil
		},
	}

	// mock FNS
	f := &mockFNS{
		FNS: FNS{
			localStrat:  mockStrat,
			remoteStrat: mockStrat,
		},
	}

	got, stream, err := f.Read(ctx, "/tmp/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil data for large file")
	}
	if stream == nil {
		t.Fatalf("expected ReadCloser for large file")
	}
	if rc.Closed {
		t.Fatalf("stream should NOT be closed for large file")
	}
}

func TestFNS_Read_IsDirectory(t *testing.T) {
	// mock strat that returns Resource of Type: "dir"
	mockStrat := &mockStrategy{
		GetInfoFunc: func(ctx context.Context, path string) (*ResourceInfo, error) {
			return &ResourceInfo{Size: 0, Type: "dir"}, nil
		},
	}

	// mock FNS to call
	f := &mockFNS{
		FNS: FNS{
			localStrat:  mockStrat,
			remoteStrat: mockStrat,
		},
	}

	_, _, err := f.Read(context.Background(), "/tmp/foo")
	if err == nil {
		t.Fatalf("expected error for directory")
	}
}

func TestFNS_Read_ReadStreamError(t *testing.T) {
	ctx := context.Background()

	mockStrat := &mockStrategy{
		ReadStreamFunc: func(ctx context.Context, path string) (io.ReadCloser, error) {
			return nil, errors.New("boom")
		},
		GetInfoFunc: func(ctx context.Context, path string) (*ResourceInfo, error) {
			return &ResourceInfo{Size: 1, Type: "file"}, nil
		},
	}

	f := &mockFNS{
		FNS: FNS{
			localStrat:  mockStrat,
			remoteStrat: mockStrat,
		},
	}

	_, _, err := f.Read(ctx, "/tmp/foo")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
}

func TestFNS_ReadStream_Remote(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Server returns large body
	largeData := make([]byte, Maxsize+1) // larger than 20 MB
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeData)
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
	if len(n) != Maxsize+1 {
		t.Errorf("ReadStream(remote) returned incorrect size: got %d, want %d", len(n), Maxsize+1)
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
	largeData := make([]byte, Maxsize+1) // larger than 20 MB

	_, err = tflarge.Write(largeData)
	if err != nil {
		t.Fatalf("failed to write to large temp file: %v", err)
		tflarge.Close()
	}
	tflarge.Close()

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
	if len(n) != Maxsize+1 {
		t.Errorf("ReadStream() returned incorrect size: got %d, want %d", len(n), Maxsize+1)
	}
}
func TestFNS_Write(t *testing.T) {
	fns := NewFNS()
	ctx := context.Background()

	// Test remote
	err := fns.Write(ctx, "http://example.com/resource", []byte("test"))
	if err == nil {
		t.Error("Write(remote) should return error for forbidden method")
	}
	tf, err := os.CreateTemp("", "write-local-*.txt")
	defer os.Remove(tf.Name())
	tf.Close()

	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Test local
	err = fns.Write(ctx, tf.Name(), []byte("test data"))
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

// Extras used for testing (mock strategies, functions)
// -----------------------------------------------------//

// snapshot returns a sorted list of paths inside root
func snapshot(t *testing.T, root string) []string {
	t.Helper()
	var result []string

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		result = append(result, rel)
		return nil
	})

	sort.Strings(result)
	return result
}

// resetSandbox removes everything except baseline items
func resetSandbox(t *testing.T, root string, baseline []string) {
	t.Helper()

	now := snapshot(t, root)

	// convert slices to sets
	baseSet := make(map[string]struct{})
	for _, b := range baseline {
		baseSet[b] = struct{}{}
	}

	for _, path := range now {
		if _, ok := baseSet[path]; !ok {
			// remove all unexpected files or dirs
			os.RemoveAll(filepath.Join(root, path))
		}
	}
}

// noopStrategy is a ResourceStrategy used for testing that returns errors for all methods and is here only so that mockStrat can implement ResourceStrategy and be used by mockFNS in all the [...]Stream() Functions
type noopStrategy struct{}

func (noopStrategy) GetInfo(ctx context.Context, path string) (*ResourceInfo, error) {
	return nil, fmt.Errorf("noop GetInfo")
}
func (noopStrategy) Exists(ctx context.Context, path string) (bool, error) {
	return false, fmt.Errorf("noop Exists")
}
func (noopStrategy) IsDir(ctx context.Context, path string) (bool, error) {
	return false, fmt.Errorf("noop IsDir")
}
func (noopStrategy) IsFile(ctx context.Context, path string) (bool, error) {
	return false, fmt.Errorf("noop IsFile")
}
func (noopStrategy) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("noop ReadStream")
}
func (noopStrategy) Write(ctx context.Context, path string, data []byte) error {
	return fmt.Errorf("noop Write")
}
func (noopStrategy) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	return fmt.Errorf("noop WriteStream")
}
func (noopStrategy) Append(ctx context.Context, path string, data []byte) error {
	return fmt.Errorf("noop Append")
}
func (noopStrategy) List(ctx context.Context, path string) ([]ResourceInfo, error) {
	return nil, fmt.Errorf("noop List")
}
func (noopStrategy) Mkdir(ctx context.Context, path string, perm os.FileMode) error {
	return fmt.Errorf("noop Mkdir")
}
func (noopStrategy) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	return fmt.Errorf("noop MkdirAll")
}
func (noopStrategy) Remove(ctx context.Context, path string) error {
	return fmt.Errorf("noop Remove")
}
func (noopStrategy) RemoveAll(ctx context.Context, path string) error {
	return fmt.Errorf("noop RemoveAll")
}
func (noopStrategy) Copy(ctx context.Context, src, dst string) error {
	return fmt.Errorf("noop Copy")
}
func (noopStrategy) Move(ctx context.Context, src, dst string) error {
	return fmt.Errorf("noop Move")
}
func (noopStrategy) Rename(ctx context.Context, src, dst string) error {
	return fmt.Errorf("noop Rename")
}
func (noopStrategy) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	return fmt.Errorf("noop Chmod")
}
func (noopStrategy) Chown(ctx context.Context, path string, uid, gid int) error {
	return fmt.Errorf("noop Chown")
}
func (noopStrategy) Download(ctx context.Context, url, dst string, progress func(int)) error {
	return fmt.Errorf("noop Download")
}
func (noopStrategy) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	return nil, fmt.Errorf("noop DownloadStream")
}
func (noopStrategy) Fetch(ctx context.Context, url string) ([]byte, error) {
	return nil, fmt.Errorf("noop Fetch")
}
func (noopStrategy) Validate(ctx context.Context, path string) error {
	return fmt.Errorf("noop Validate")
}

// mockRC is a mock ReadCloser for testing
type mockRC struct {
	io.Reader
	Closed bool
}

// Close marks the ReadCloser as closed
func (m *mockRC) Close() error {
	m.Closed = true
	return nil
}

// mockStrategy is a mock ResourceStrategy for testing
type mockStrategy struct {
	noopStrategy

	GetInfoFunc        func(ctx context.Context, path string) (*ResourceInfo, error)
	ReadStreamFunc     func(ctx context.Context, path string) (io.ReadCloser, error)
	WriteStreamFunc    func(ctx context.Context, path string, reader io.Reader) error
	DownloadStreamFunc func(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error)
}

// GetInfo calls the mocked GetInfoFunc
func (m *mockStrategy) GetInfo(ctx context.Context, path string) (*ResourceInfo, error) {
	return m.GetInfoFunc(ctx, path)
}

// Read calls the mocked ReadStreamFunc
func (m *mockStrategy) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	return m.ReadStreamFunc(ctx, path)
}

// WriteStream calls the mocked WriteStreamFunc
func (m *mockStrategy) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	return m.WriteStreamFunc(ctx, path, reader)
}

// DownloadStream calls the mocked DownloadStreamFunc
func (m *mockStrategy) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	return m.DownloadStreamFunc(ctx, url, progress)
}

// mockFNS is a mock FNS for testing
type mockFNS struct {
	FNS
}
