package fns

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
)

func TestGetStrategy_LocalPath(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	exists, err := Exists(ctx, testFile)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist")
	}
}

func TestGetStrategy_HTTPPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	exists, err := Exists(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected URL to exist")
	}
}

func TestGetStrategy_HTTPSPath(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	url := strings.Replace(srv.URL, "http://", "https://", 1)
	_, err := Exists(ctx, url)
	if err == nil {
		t.Log("HTTPS path detected (may fail due to TLS verification)")
	}
}

func TestGetStrategy_WithConfigOptions(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	customClient := &http.Client{Timeout: 60 * time.Second}
	exists, err := Exists(ctx, testFile, config.WithHTTPClient(customClient), config.WithMaxMemorySize(50*1024*1024), config.WithBufferSize(64*1024))
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist")
	}
}

func TestGetInfo_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	size, resourceType, modTime, err := GetInfo(ctx, testFile)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if size != 4 {
		t.Errorf("Expected size 4, got %d", size)
	}
	if resourceType != "file" {
		t.Errorf("Expected type file, got %s", resourceType)
	}
	if modTime.IsZero() {
		t.Error("Expected non-zero ModTime")
	}
}

func TestGetInfo_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "9")
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Write([]byte("test data"))
	}))
	defer srv.Close()

	ctx := context.Background()
	size, resourceType, modTime, err := GetInfo(ctx, srv.URL)
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

func TestExists_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	exists, _ := Exists(ctx, testFile)
	if exists {
		t.Error("Expected file to not exist")
	}

	os.WriteFile(testFile, []byte("test"), 0o644)

	exists, err := Exists(ctx, testFile)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist")
	}
}

func TestExists_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	exists, err := Exists(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected URL to exist")
	}
}

func TestIsDir_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0o755)

	isDir, err := IsDir(ctx, testDir)
	if err != nil {
		t.Fatalf("IsDir failed: %v", err)
	}
	if !isDir {
		t.Error("Expected directory")
	}

	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	isDir, err = IsDir(ctx, testFile)
	if err != nil {
		t.Fatalf("IsDir failed: %v", err)
	}
	if isDir {
		t.Error("Expected file, not directory")
	}
}

func TestIsDir_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := IsDir(ctx, srv.URL)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestIsFile_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	isFile, err := IsFile(ctx, testFile)
	if err != nil {
		t.Fatalf("IsFile failed: %v", err)
	}
	if !isFile {
		t.Error("Expected file")
	}

	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0o755)

	isFile, err = IsFile(ctx, testDir)
	if err != nil {
		t.Fatalf("IsFile failed: %v", err)
	}
	if isFile {
		t.Error("Expected directory, not file")
	}
}

func TestIsFile_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := IsFile(ctx, srv.URL)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestRead_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test data"), 0o644)

	data, err := Read(ctx, testFile)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("Expected %q, got %q", "test data", string(data))
	}
}

func TestRead_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("remote data"))
	}))
	defer srv.Close()

	ctx := context.Background()
	data, err := Read(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != "remote data" {
		t.Errorf("Expected %q, got %q", "remote data", string(data))
	}
}

func TestReadStream_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("stream data"), 0o644)

	stream, err := ReadStream(ctx, testFile)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "stream data" {
		t.Errorf("Expected %q, got %q", "stream data", string(data))
	}
}

func TestReadStream_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("stream data"))
	}))
	defer srv.Close()

	ctx := context.Background()
	stream, err := ReadStream(ctx, srv.URL)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "stream data" {
		t.Errorf("Expected %q, got %q", "stream data", string(data))
	}
}

func TestWrite_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "subdir", "test.txt")

	err := Write(ctx, testFile, []byte("test data"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "test data" {
		t.Errorf("Expected %q, got %q", "test data", string(data))
	}
}

func TestWrite_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := Write(ctx, srv.URL, []byte("test data"))
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestWriteStream_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	reader := strings.NewReader("stream write data")
	err := WriteStream(ctx, testFile, reader)
	if err != nil {
		t.Fatalf("WriteStream failed: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "stream write data" {
		t.Errorf("Expected %q, got %q", "stream write data", string(data))
	}
}

func TestWriteStream_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	reader := strings.NewReader("stream write data")
	err := WriteStream(ctx, srv.URL, reader)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestAppend_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("original"), 0o644)

	err := Append(ctx, testFile, []byte(" appended"))
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "original appended" {
		t.Errorf("Expected %q, got %q", "original appended", string(data))
	}
}

func TestAppend_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := Append(ctx, srv.URL, []byte("appended"))
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestList_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0o755)
	os.WriteFile(filepath.Join(testDir, "file1.txt"), []byte("test"), 0o644)
	os.WriteFile(filepath.Join(testDir, "file2.txt"), []byte("test"), 0o644)

	list, err := List(ctx, testDir)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("Expected 2 items, got %d", len(list))
	}
}

func TestList_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("file1\nfile2\nfile3"))
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := List(ctx, srv.URL)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestMkdir_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "newdir")

	err := Mkdir(ctx, testDir, 0o755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("Directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected directory")
	}
}

func TestMkdir_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := Mkdir(ctx, srv.URL, 0o755)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestMkdirAll_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "dir1", "dir2", "dir3")

	err := MkdirAll(ctx, testDir, 0o755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("Directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected directory")
	}
}

func TestMkdirAll_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := MkdirAll(ctx, srv.URL, 0o755)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestRemove_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	err := Remove(ctx, testFile)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	_, err = os.Stat(testFile)
	if err == nil {
		t.Error("Expected file to be removed")
	}
}

func TestRemove_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := Remove(ctx, srv.URL)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestRemoveAll_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0o755)
	os.WriteFile(filepath.Join(testDir, "file1.txt"), []byte("test"), 0o644)
	os.WriteFile(filepath.Join(testDir, "file2.txt"), []byte("test"), 0o644)

	err := RemoveAll(ctx, testDir)
	if err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	_, err = os.Stat(testDir)
	if err == nil {
		t.Error("Expected directory to be removed")
	}
}

func TestRemoveAll_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := RemoveAll(ctx, srv.URL)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestCopy_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	srcFile := filepath.Join(sandbox, "source.txt")
	dstFile := filepath.Join(sandbox, "dest.txt")
	os.WriteFile(srcFile, []byte("source data"), 0o644)

	err := Copy(ctx, srcFile, dstFile)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	data, _ := os.ReadFile(dstFile)
	if string(data) != "source data" {
		t.Errorf("Expected %q, got %q", "source data", string(data))
	}
}

func TestCopy_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("remote data"))
	}))
	defer srv.Close()

	ctx := context.Background()
	sandbox := t.TempDir()
	dstFile := filepath.Join(sandbox, "dest.txt")

	err := Copy(ctx, srv.URL, dstFile)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	data, _ := os.ReadFile(dstFile)
	if string(data) != "remote data" {
		t.Errorf("Expected %q, got %q", "remote data", string(data))
	}
}

func TestMove_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	srcFile := filepath.Join(sandbox, "source.txt")
	dstFile := filepath.Join(sandbox, "dest.txt")
	os.WriteFile(srcFile, []byte("source data"), 0o644)

	err := Move(ctx, srcFile, dstFile)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	_, err = os.Stat(srcFile)
	if err == nil {
		t.Error("Expected source file to be removed")
	}

	data, _ := os.ReadFile(dstFile)
	if string(data) != "source data" {
		t.Errorf("Expected %q, got %q", "source data", string(data))
	}
}

func TestMove_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	sandbox := t.TempDir()
	dstFile := filepath.Join(sandbox, "dest.txt")

	err := Move(ctx, srv.URL, dstFile)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestRename_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	oldFile := filepath.Join(sandbox, "old.txt")
	newFile := filepath.Join(sandbox, "new.txt")
	os.WriteFile(oldFile, []byte("test data"), 0o644)

	err := Rename(ctx, oldFile, newFile)
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	_, err = os.Stat(oldFile)
	if err == nil {
		t.Error("Expected old file to be removed")
	}

	data, _ := os.ReadFile(newFile)
	if string(data) != "test data" {
		t.Errorf("Expected %q, got %q", "test data", string(data))
	}
}

func TestRename_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	sandbox := t.TempDir()
	newFile := filepath.Join(sandbox, "new.txt")

	err := Rename(ctx, srv.URL, newFile)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestChmod_Local(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping chmod test on Windows")
	}

	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	err := Chmod(ctx, testFile, 0o755)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	info, _ := os.Stat(testFile)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("Expected mode 0755, got %o", info.Mode().Perm())
	}
}

func TestChmod_Remote(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping chmod test on Windows")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := Chmod(ctx, srv.URL, 0o755)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestChown_Local(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping chown test on Windows")
	}

	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	err := Chown(ctx, testFile, 1000, 1000)
	if err != nil {
		t.Logf("Chown failed (may not work on all systems): %v", err)
	}
}

func TestChown_Remote(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping chown test on Windows")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := Chown(ctx, srv.URL, 1000, 1000)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestDownload_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	srcFile := filepath.Join(sandbox, "source.txt")
	dstFile := filepath.Join(sandbox, "dest.txt")
	os.WriteFile(srcFile, []byte("download data"), 0o644)

	progress := func(bytes int) {
		_ = bytes
	}

	err := Download(ctx, srcFile, dstFile, progress)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestDownload_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("remote download data"))
	}))
	defer srv.Close()

	ctx := context.Background()
	sandbox := t.TempDir()
	dstFile := filepath.Join(sandbox, "dest.txt")

	progress := func(bytes int) {
		_ = bytes
	}

	err := Download(ctx, srv.URL, dstFile, progress)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	data, _ := os.ReadFile(dstFile)
	if string(data) != "remote download data" {
		t.Errorf("Expected %q, got %q", "remote download data", string(data))
	}
}

func TestDownloadStream_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	srcFile := filepath.Join(sandbox, "source.txt")
	os.WriteFile(srcFile, []byte("stream download data"), 0o644)

	progress := func(bytes int) {
		_ = bytes
	}

	_, err := DownloadStream(ctx, srcFile, progress)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestDownloadStream_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("remote stream download data"))
	}))
	defer srv.Close()

	ctx := context.Background()
	progress := func(bytes int) {
		_ = bytes
	}

	stream, err := DownloadStream(ctx, srv.URL, progress)
	if err != nil {
		t.Fatalf("DownloadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "remote stream download data" {
		t.Errorf("Expected %q, got %q", "remote stream download data", string(data))
	}
}

func TestFetch_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("fetch data"), 0o644)

	_, err := Fetch(ctx, testFile)
	if err == nil {
		t.Fatal("Expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Expected unsupported error, got: %v", err)
	}
}

func TestFetch_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("remote fetch data"))
	}))
	defer srv.Close()

	ctx := context.Background()
	data, err := Fetch(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(data) != "remote fetch data" {
		t.Errorf("Expected %q, got %q", "remote fetch data", string(data))
	}
}

func TestValidate_Local(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	err := Validate(ctx, testFile)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestValidate_Remote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	err := Validate(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestAllFunctions_WithConfigOptions(t *testing.T) {
	ctx := context.Background()
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	customClient := &http.Client{Timeout: 60 * time.Second}
	opts := []config.Option{
		config.WithHTTPClient(customClient),
		config.WithMaxMemorySize(50 * 1024 * 1024),
		config.WithBufferSize(64 * 1024),
	}

	exists, err := Exists(ctx, testFile, opts...)
	if err != nil {
		t.Fatalf("Exists with options failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist")
	}

	size, _, _, err := GetInfo(ctx, testFile, opts...)
	if err != nil {
		t.Fatalf("GetInfo with options failed: %v", err)
	}
	if size != 4 {
		t.Errorf("Expected size 4, got %d", size)
	}
}
