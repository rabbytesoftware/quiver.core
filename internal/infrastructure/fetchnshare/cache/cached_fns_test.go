package cache

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare"
)

func TestCachedFNS_Read(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test data"), 0644)

	ctx := context.Background()

	data1, _, err := cached.Read(ctx, testFile)
	if err != nil {
		t.Fatalf("First read failed: %v", err)
	}
	if string(data1) != "test data" {
		t.Errorf("Expected %q, got %q", "test data", string(data1))
	}

	os.WriteFile(testFile, []byte("modified data"), 0644)

	data2, _, err := cached.Read(ctx, testFile)
	if err != nil {
		t.Fatalf("Second read failed: %v", err)
	}
	if string(data2) != "test data" {
		t.Errorf("Expected cached %q, got %q", "test data", string(data2))
	}
}

func TestCachedFNS_Write_InvalidatesCache(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("original"), 0644)

	ctx := context.Background()

	data1, _, _ := cached.Read(ctx, testFile)
	if string(data1) != "original" {
		t.Errorf("Expected %q, got %q", "original", string(data1))
	}

	err := cached.Write(ctx, testFile, []byte("updated"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data2, _, _ := cached.Read(ctx, testFile)
	if string(data2) != "updated" {
		t.Errorf("Expected cache invalidation, got cached %q", string(data2))
	}
}

func TestCachedFNS_ReadStream(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("stream data"), 0644)

	ctx := context.Background()

	stream, err := cached.ReadStream(ctx, testFile)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "stream data" {
		t.Errorf("Expected %q, got %q", "stream data", string(data))
	}
}

func TestCachedFNS_Delegation(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	ctx := context.Background()

	exists, err := cached.Exists(ctx, sandbox)
	if err != nil {
		t.Errorf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected sandbox to exist")
	}

	isDir, err := cached.IsDir(ctx, sandbox)
	if err != nil {
		t.Errorf("IsDir failed: %v", err)
	}
	if !isDir {
		t.Error("Expected sandbox to be directory")
	}
}

func TestCachedFNS_GetInfo(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	info, err := cached.GetInfo(ctx, testFile)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if info.Size != 4 {
		t.Errorf("Expected size 4, got %d", info.Size)
	}
}

func TestCachedFNS_IsFile(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	isFile, err := cached.IsFile(ctx, testFile)
	if err != nil {
		t.Fatalf("IsFile failed: %v", err)
	}
	if !isFile {
		t.Error("Expected to be file")
	}
}

func TestCachedFNS_WriteStream_InvalidatesCache(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("original"), 0644)

	ctx := context.Background()

	cached.Read(ctx, testFile)

	err := cached.WriteStream(ctx, testFile, strings.NewReader("updated"))
	if err != nil {
		t.Fatalf("WriteStream failed: %v", err)
	}

	data, _, _ := cached.Read(ctx, testFile)
	if string(data) != "updated" {
		t.Errorf("Expected cache invalidation")
	}
}

func TestCachedFNS_Append_InvalidatesCache(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("original"), 0644)

	ctx := context.Background()

	cached.Read(ctx, testFile)

	err := cached.Append(ctx, testFile, []byte(" appended"))
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	data, _, _ := cached.Read(ctx, testFile)
	if string(data) != "original appended" {
		t.Errorf("Expected %q, got %q", "original appended", string(data))
	}
}

func TestCachedFNS_List(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	os.WriteFile(filepath.Join(sandbox, "file1.txt"), []byte("test"), 0644)

	ctx := context.Background()

	list, err := cached.List(ctx, sandbox)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 item, got %d", len(list))
	}
}

func TestCachedFNS_DirectoryOperations(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	ctx := context.Background()

	newDir := filepath.Join(sandbox, "newdir")
	err := cached.Mkdir(ctx, newDir, 0755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	nestedDir := filepath.Join(sandbox, "a", "b", "c")
	err = cached.MkdirAll(ctx, nestedDir, 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err = cached.Remove(ctx, testFile)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	err = cached.RemoveAll(ctx, newDir)
	if err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}
}

func TestCachedFNS_FileOperations(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	ctx := context.Background()

	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("copy test"), 0644)

	err := cached.Copy(ctx, src, dst)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	src2 := filepath.Join(sandbox, "src2.txt")
	dst2 := filepath.Join(sandbox, "dst2.txt")
	os.WriteFile(src2, []byte("move test"), 0644)

	cached.Read(ctx, src2)

	err = cached.Move(ctx, src2, dst2)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	src3 := filepath.Join(sandbox, "src3.txt")
	dst3 := filepath.Join(sandbox, "dst3.txt")
	os.WriteFile(src3, []byte("rename test"), 0644)

	cached.Read(ctx, src3)

	err = cached.Rename(ctx, src3, dst3)
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
}

func TestCachedFNS_Chmod_Chown(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	err := cached.Chmod(ctx, testFile, 0755)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	err = cached.Chown(ctx, testFile, os.Getuid(), os.Getgid())
	if err != nil && !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("Chown failed: %v", err)
	}
}

func TestCachedFNS_Download(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "downloaded.txt")

	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4")
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	err := cached.Download(ctx, srv.URL, dst, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
}

func TestCachedFNS_DownloadStream(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("stream"))
	}))
	defer srv.Close()

	stream, err := cached.DownloadStream(ctx, srv.URL, nil)
	if err != nil {
		t.Fatalf("DownloadStream failed: %v", err)
	}
	if stream != nil {
		stream.Close()
	}
}

func TestCachedFNS_Fetch_Caching(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	ctx := context.Background()
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("fetch data"))
	}))
	defer srv.Close()

	data1, err := cached.Fetch(ctx, srv.URL)
	if err != nil {
		t.Fatalf("First fetch failed: %v", err)
	}

	data2, err := cached.Fetch(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}

	if string(data1) != string(data2) {
		t.Error("Expected same data from cache")
	}

	if callCount > 1 {
		t.Errorf("Expected 1 HTTP call (cached), got %d", callCount)
	}
}

func TestCachedFNS_Resolve(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	path, resourceType, err := cached.Resolve(ctx, testFile)
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

func TestCachedFNS_Validate(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	ctx := context.Background()

	err := cached.Validate(ctx, testFile)
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestCachedFNS_TempFile_TempDir(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	ctx := context.Background()

	tempFile, err := cached.TempFile(ctx, "cached-test-*")
	if err != nil {
		t.Fatalf("TempFile failed: %v", err)
	}
	defer os.Remove(tempFile)

	tempDir, err := cached.TempDir(ctx, "cached-test-*")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if tempFile == "" {
		t.Error("Expected non-empty temp file path")
	}
	if tempDir == "" {
		t.Error("Expected non-empty temp dir path")
	}
}

func TestCachedFNS_Walk(t *testing.T) {
	core := fetchnshare.NewFNS()
	cached := NewCachedFNS(core, 1*time.Hour)

	sandbox := t.TempDir()
	os.WriteFile(filepath.Join(sandbox, "test.txt"), []byte("test"), 0644)

	ctx := context.Background()

	var count int
	walkFn := func(path string, info fetchnshare.ResourceInfo, err error) error {
		count++
		return nil
	}

	err := cached.Walk(ctx, sandbox, walkFn)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	if count == 0 {
		t.Error("Expected Walk to visit at least one path")
	}
}
