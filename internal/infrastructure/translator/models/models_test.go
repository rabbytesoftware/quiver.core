package models

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	fns "github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare"
)

func TestResourceInfo(t *testing.T) {
	// Test that ResourceInfo struct can be created and used
	info := fns.ResourceInfo{
		Path:    "test",
		Size:    1024,
		Type:    fns.ResourceTypeFile,
		ModTime: time.Now(),
	}

	if info.Path != "test" {
		t.Errorf("Expected Path to be 'test', got %s", info.Path)
	}

	if info.Size != 1024 {
		t.Errorf("Expected Size to be 1024, got %d", info.Size)
	}

	if info.Type != fns.ResourceTypeFile {
		t.Error("Expected Type to be ResourceTypeFile")
	}

	if time.Since(info.ModTime) > time.Minute {
		t.Error("Expected ModTime to be recent")
	}
}

func TestResourceType(t *testing.T) {
	// Test that ResourceType can be used as a string
	rt := fns.ResourceTypeFile
	if rt != "file" {
		t.Errorf("Expected ResourceType to be 'file', got %s", rt)
	}

	// Test different resource types
	types := []fns.ResourceType{fns.ResourceTypeFile, fns.ResourceTypeDir}
	for _, rt := range types {
		if string(rt) == "" {
			t.Error("ResourceType should not be empty")
		}
	}
}

func TestFNSInterface(t *testing.T) {
	// Test that FNSInterface can be referenced
	// We can't easily test the interface without a concrete implementation,
	// but we can verify the interface definition exists and is valid

	// This test mainly verifies that the interface is properly defined
	// and can be used in type assertions
	var _ fns.FNSInterface = (*mockFNS)(nil)
}

// mockFNS is a mock implementation for testing the FNSInterface
type mockFNS struct{}

func (m *mockFNS) GetInfo(ctx context.Context, path string) (*fns.ResourceInfo, error) {
	return nil, nil
}

func (m *mockFNS) Exists(ctx context.Context, path string) (bool, error) {
	return false, nil
}

func (m *mockFNS) IsDir(ctx context.Context, path string) (bool, error) {
	return false, nil
}

func (m *mockFNS) IsFile(ctx context.Context, path string) (bool, error) {
	return false, nil
}

func (m *mockFNS) Read(ctx context.Context, path string) ([]byte, io.ReadCloser, error) {
	return nil, nil, nil
}

func (m *mockFNS) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockFNS) Write(ctx context.Context, path string, data []byte) error {
	return nil
}

func (m *mockFNS) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	return nil
}

func (m *mockFNS) Append(ctx context.Context, path string, data []byte) error {
	return nil
}

func (m *mockFNS) List(ctx context.Context, path string) ([]fns.ResourceInfo, error) {
	return nil, nil
}

func (m *mockFNS) Mkdir(ctx context.Context, path string, perm os.FileMode) error {
	return nil
}

func (m *mockFNS) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	return nil
}

func (m *mockFNS) Remove(ctx context.Context, path string) error {
	return nil
}

func (m *mockFNS) RemoveAll(ctx context.Context, path string) error {
	return nil
}

func (m *mockFNS) Copy(ctx context.Context, src, dst string) error {
	return nil
}

func (m *mockFNS) Move(ctx context.Context, src, dst string) error {
	return nil
}

func (m *mockFNS) Rename(ctx context.Context, src, dst string) error {
	return nil
}

func (m *mockFNS) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	return nil
}

func (m *mockFNS) Chown(ctx context.Context, path string, uid, gid int) error {
	return nil
}

func (m *mockFNS) Download(ctx context.Context, url, dst string, progress func(int)) error {
	return nil
}

func (m *mockFNS) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockFNS) Fetch(ctx context.Context, url string) ([]byte, error) {
	return nil, nil
}

func (m *mockFNS) CacheGet(ctx context.Context, key string) ([]byte, error) {
	return nil, nil
}

func (m *mockFNS) CacheSet(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	return nil
}

func (m *mockFNS) CacheDelete(ctx context.Context, key string) error {
	return nil
}

func (m *mockFNS) CacheClear(ctx context.Context) error {
	return nil
}

func (m *mockFNS) Resolve(ctx context.Context, path string) (string, fns.ResourceType, error) {
	return "", "", nil
}

func (m *mockFNS) Validate(ctx context.Context, path string) error {
	return nil
}

func (m *mockFNS) TempFile(ctx context.Context, pattern string) (string, error) {
	return "", nil
}

func (m *mockFNS) TempDir(ctx context.Context, pattern string) (string, error) {
	return "", nil
}

func (m *mockFNS) Walk(ctx context.Context, root string, fn func(path string, info fns.ResourceInfo, err error) error) error {
	return nil
}

func TestFNSInterfaceMethods(t *testing.T) {
	mock := &mockFNS{}
	ctx := context.Background()

	// Test that all interface methods can be called
	_, err := mock.GetInfo(ctx, "test")
	if err != nil {
		t.Errorf("GetInfo() returned error: %v", err)
	}

	_, err = mock.Exists(ctx, "test")
	if err != nil {
		t.Errorf("Exists() returned error: %v", err)
	}

	_, err = mock.IsDir(ctx, "test")
	if err != nil {
		t.Errorf("IsDir() returned error: %v", err)
	}

	_, err = mock.IsFile(ctx, "test")
	if err != nil {
		t.Errorf("IsFile() returned error: %v", err)
	}

	_, _, err = mock.Read(ctx, "test")
	if err != nil {
		t.Errorf("Read() returned error: %v", err)
	}

	_, err = mock.ReadStream(ctx, "test")
	if err != nil {
		t.Errorf("ReadStream() returned error: %v", err)
	}

	err = mock.Write(ctx, "test", []byte("data"))
	if err != nil {
		t.Errorf("Write() returned error: %v", err)
	}

	err = mock.WriteStream(ctx, "test", nil)
	if err != nil {
		t.Errorf("WriteStream() returned error: %v", err)
	}

	err = mock.Append(ctx, "test", []byte("data"))
	if err != nil {
		t.Errorf("Append() returned error: %v", err)
	}

	_, err = mock.List(ctx, "test")
	if err != nil {
		t.Errorf("List() returned error: %v", err)
	}

	err = mock.Mkdir(ctx, "test", 0755)
	if err != nil {
		t.Errorf("Mkdir() returned error: %v", err)
	}

	err = mock.MkdirAll(ctx, "test", 0755)
	if err != nil {
		t.Errorf("MkdirAll() returned error: %v", err)
	}

	err = mock.Remove(ctx, "test")
	if err != nil {
		t.Errorf("Remove() returned error: %v", err)
	}

	err = mock.RemoveAll(ctx, "test")
	if err != nil {
		t.Errorf("RemoveAll() returned error: %v", err)
	}

	err = mock.Copy(ctx, "src", "dst")
	if err != nil {
		t.Errorf("Copy() returned error: %v", err)
	}

	err = mock.Move(ctx, "src", "dst")
	if err != nil {
		t.Errorf("Move() returned error: %v", err)
	}

	err = mock.Rename(ctx, "src", "dst")
	if err != nil {
		t.Errorf("Rename() returned error: %v", err)
	}

	err = mock.Chmod(ctx, "test", 0755)
	if err != nil {
		t.Errorf("Chmod() returned error: %v", err)
	}

	err = mock.Chown(ctx, "test", 1000, 1000)
	if err != nil {
		t.Errorf("Chown() returned error: %v", err)
	}

	progress := func(int) {}
	err = mock.Download(ctx, "http://example.com", "dst", progress)
	if err != nil {
		t.Errorf("Download() returned error: %v", err)
	}

	_, err = mock.DownloadStream(ctx, "http://example.com", progress)
	if err != nil {
		t.Errorf("DownloadStream() returned error: %v", err)
	}

	_, err = mock.Fetch(ctx, "http://example.com")
	if err != nil {
		t.Errorf("Fetch() returned error: %v", err)
	}

	_, err = mock.CacheGet(ctx, "key")
	if err != nil {
		t.Errorf("CacheGet() returned error: %v", err)
	}

	err = mock.CacheSet(ctx, "key", []byte("data"), time.Hour)
	if err != nil {
		t.Errorf("CacheSet() returned error: %v", err)
	}

	err = mock.CacheDelete(ctx, "key")
	if err != nil {
		t.Errorf("CacheDelete() returned error: %v", err)
	}

	err = mock.CacheClear(ctx)
	if err != nil {
		t.Errorf("CacheClear() returned error: %v", err)
	}

	_, _, err = mock.Resolve(ctx, "test")
	if err != nil {
		t.Errorf("Resolve() returned error: %v", err)
	}

	err = mock.Validate(ctx, "test")
	if err != nil {
		t.Errorf("Validate() returned error: %v", err)
	}

	_, err = mock.TempFile(ctx, "test-*")
	if err != nil {
		t.Errorf("TempFile() returned error: %v", err)
	}

	_, err = mock.TempDir(ctx, "test-*")
	if err != nil {
		t.Errorf("TempDir() returned error: %v", err)
	}

	walkFn := func(path string, info fns.ResourceInfo, err error) error {
		return nil
	}
	err = mock.Walk(ctx, "test", walkFn)
	if err != nil {
		t.Errorf("Walk() returned error: %v", err)
	}
}
