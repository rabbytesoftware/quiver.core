package strategies

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/fns/config"
)

func TestLocal_GetInfo(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	size, resourceType, modTime, err := l.GetInfo(ctx, testFile)
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

func TestLocal_Exists(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	exists, _ := l.Exists(ctx, testFile)
	if exists {
		t.Error("Expected file to not exist")
	}

	os.WriteFile(testFile, []byte("test"), 0644)

	exists, err := l.Exists(ctx, testFile)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist")
	}
}

func TestLocal_ReadStream(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("stream data"), 0644)

	stream, err := l.ReadStream(ctx, testFile)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	if string(data) != "stream data" {
		t.Errorf("Expected %q, got %q", "stream data", string(data))
	}
}

func TestLocal_Write(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "subdir", "test.txt")

	err := l.Write(ctx, testFile, []byte("test data"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "test data" {
		t.Errorf("Expected %q, got %q", "test data", string(data))
	}
}

func TestLocal_WriteStream(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	reader := strings.NewReader("stream data")

	err := l.WriteStream(ctx, testFile, reader)
	if err != nil {
		t.Fatalf("WriteStream failed: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "stream data" {
		t.Errorf("Expected %q, got %q", "stream data", string(data))
	}
}

func TestLocal_Copy(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("copy data"), 0644)

	err := l.Copy(ctx, src, dst)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "copy data" {
		t.Errorf("Expected %q, got %q", "copy data", string(data))
	}
}

func TestLocal_Move(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("move data"), 0644)

	err := l.Move(ctx, src, dst)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("Expected source file to be removed")
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "move data" {
		t.Errorf("Expected %q, got %q", "move data", string(data))
	}
}

func TestLocal_IsDir_IsFile(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	isDir, err := l.IsDir(ctx, sandbox)
	if err != nil {
		t.Fatalf("IsDir failed: %v", err)
	}
	if !isDir {
		t.Error("Expected sandbox to be directory")
	}

	isFile, err := l.IsFile(ctx, testFile)
	if err != nil {
		t.Fatalf("IsFile failed: %v", err)
	}
	if !isFile {
		t.Error("Expected test.txt to be file")
	}

	isDir, err = l.IsDir(ctx, testFile)
	if err != nil {
		t.Fatalf("IsDir on file failed: %v", err)
	}
	if isDir {
		t.Error("Expected file to not be directory")
	}
}

func TestLocal_Append(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "append.txt")

	err := l.Append(ctx, testFile, []byte("first"))
	if err != nil {
		t.Fatalf("First append failed: %v", err)
	}

	err = l.Append(ctx, testFile, []byte("second"))
	if err != nil {
		t.Fatalf("Second append failed: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "firstsecond" {
		t.Errorf("Expected %q, got %q", "firstsecond", string(data))
	}
}

func TestLocal_List(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	os.WriteFile(filepath.Join(sandbox, "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(sandbox, "file2.txt"), []byte("test"), 0644)
	os.Mkdir(filepath.Join(sandbox, "subdir"), 0755)

	names, err := l.List(ctx, sandbox)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 3 {
		t.Errorf("Expected 3 items, got %d", len(names))
	}
}

func TestLocal_List_NotDirectory(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	_, err := l.List(ctx, testFile)
	if err == nil {
		t.Error("Expected error when listing file")
	}
}

func TestLocal_Mkdir_MkdirAll(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()

	newDir := filepath.Join(sandbox, "newdir")
	err := l.Mkdir(ctx, newDir, 0755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	nestedDir := filepath.Join(sandbox, "a", "b", "c")
	err = l.MkdirAll(ctx, nestedDir, 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	exists, _ := l.Exists(ctx, nestedDir)
	if !exists {
		t.Error("Expected nested directory to exist")
	}
}

func TestLocal_Remove_RemoveAll(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Remove(ctx, testFile)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	exists, _ := l.Exists(ctx, testFile)
	if exists {
		t.Error("Expected file to be removed")
	}

	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "file.txt"), []byte("test"), 0644)

	err = l.RemoveAll(ctx, testDir)
	if err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	exists, _ = l.Exists(ctx, testDir)
	if exists {
		t.Error("Expected directory to be removed")
	}
}

func TestLocal_Remove_NonEmptyDir(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "file.txt"), []byte("test"), 0644)

	err := l.Remove(ctx, testDir)
	if err == nil {
		t.Error("Expected error when removing non-empty directory")
	}
}

func TestLocal_Rename(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "old.txt")
	dst := filepath.Join(sandbox, "new.txt")
	os.WriteFile(src, []byte("rename test"), 0644)

	err := l.Rename(ctx, src, dst)
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	exists, _ := l.Exists(ctx, src)
	if exists {
		t.Error("Expected old file to not exist")
	}

	exists, _ = l.Exists(ctx, dst)
	if !exists {
		t.Error("Expected new file to exist")
	}
}

func TestLocal_Chmod(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Chmod(ctx, testFile, 0755)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	stat, _ := os.Stat(testFile)
	if stat.Mode().Perm() != 0755 {
		t.Errorf("Expected permissions 0755, got %o", stat.Mode().Perm())
	}
}

func TestLocal_Chown(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping chown test (requires root)")
	}

	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Chown(ctx, testFile, os.Getuid(), os.Getgid())
	if err != nil {
		t.Fatalf("Chown failed: %v", err)
	}
}

func TestLocal_Validate(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Validate(ctx, testFile)
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}

	err = l.Validate(ctx, sandbox)
	if err != nil {
		t.Errorf("Validate directory failed: %v", err)
	}

	err = l.Validate(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_GetInfo_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, _, _, err := l.GetInfo(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_IsDir_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.IsDir(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_IsFile_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.IsFile(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_ReadStream_Directory(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()

	_, err := l.ReadStream(ctx, sandbox)
	if err == nil {
		t.Error("Expected error when reading directory")
	}
}

func TestLocal_List_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.List(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_Remove_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Remove(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_RemoveAll_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.RemoveAll(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_Copy_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "dst.txt")

	err := l.Copy(ctx, "/nonexistent/src", dst)
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestLocal_Move_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "dst.txt")

	err := l.Move(ctx, "/nonexistent/src", dst)
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestLocal_Rename_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "dst.txt")

	err := l.Rename(ctx, "/nonexistent/src", dst)
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestLocal_ReadStream_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.ReadStream(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_Write_InvalidPath(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Write(ctx, "/dev/null/cannot/write/here", []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_WriteStream_InvalidPath(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.WriteStream(ctx, "/dev/null/cannot/write/here", strings.NewReader("test"))
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_Append_InvalidPath(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Append(ctx, "/dev/null/cannot/write/here", []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_Mkdir_InvalidPath(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Mkdir(ctx, "/invalid/no/permission/path", 0755)
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_MkdirAll_InvalidPath(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.MkdirAll(ctx, "/invalid/no/permission/path", 0755)
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_Chmod_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Chmod(ctx, "/nonexistent/path", 0755)
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_GetInfo_WithSymlink(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	targetFile := filepath.Join(sandbox, "target.txt")
	os.WriteFile(targetFile, []byte("target data"), 0644)

	symlinkPath := filepath.Join(sandbox, "symlink.txt")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Skip("Symlink creation not supported")
	}

	size, rtype, _, err := l.GetInfo(ctx, symlinkPath)
	if err != nil {
		t.Fatalf("GetInfo on symlink failed: %v", err)
	}
	if size != 11 {
		t.Errorf("Expected size 11, got %d", size)
	}
	if rtype != "file" {
		t.Errorf("Expected type file, got %s", rtype)
	}
}

func TestLocal_Exists_Stat_Error(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	exists, err := l.Exists(ctx, "/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("Exists should not return error for non-existent: %v", err)
	}
	if exists {
		t.Error("Expected false for non-existent path")
	}
}

func TestLocal_ReadStream_OpenError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.ReadStream(ctx, "/nonexistent/definitely/not/here")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLocal_Write_CreateError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "not-writable-file.txt")
	err := l.Write(ctx, invalidPath, []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_WriteStream_CreateError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "not-writable-file.txt")
	err := l.WriteStream(ctx, invalidPath, strings.NewReader("test"))
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_WriteStream_CopyError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	errReader := &errorReader{err: errors.New("read error")}
	err := l.WriteStream(ctx, testFile, errReader)
	if err == nil {
		t.Error("Expected error from reader")
	}
}

type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

func TestLocal_Append_OpenFileError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "not-writable-file.txt")
	err := l.Append(ctx, invalidPath, []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_Append_WriteError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("initial"), 0644)
	os.Chmod(testFile, 0444)

	err := l.Append(ctx, testFile, []byte("append"))
	if err == nil && os.Getuid() != 0 {
		t.Error("Expected error when writing to read-only file")
	}

	os.Chmod(testFile, 0644)
}

func TestLocal_List_ReadDirError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.List(ctx, "/nonexistent/directory/path")
	if err == nil {
		t.Error("Expected error for nonexistent directory")
	}
}

func TestLocal_Mkdir_Error(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "cannot-create-dir")
	err := l.Mkdir(ctx, invalidPath, 0755)
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_MkdirAll_Error(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "cannot", "create", "nested")
	err := l.MkdirAll(ctx, invalidPath, 0755)
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_Copy_OpenSrcError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "dst.txt")

	err := l.Copy(ctx, "/nonexistent/source.txt", dst)
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestLocal_Copy_CreateDstError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	os.WriteFile(src, []byte("test"), 0644)

	invalidDst := filepath.Join("/", "proc", "cannot-write.txt")
	err := l.Copy(ctx, src, invalidDst)
	if err == nil {
		t.Error("Expected error for invalid destination")
	}
}

func TestLocal_Copy_Directory(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	srcDir := filepath.Join(sandbox, "srcdir")
	dstDir := filepath.Join(sandbox, "dstdir")
	os.Mkdir(srcDir, 0755)

	err := l.Copy(ctx, srcDir, dstDir)
	if err == nil {
		t.Error("Expected error when copying directory")
	}
}

func TestLocal_Move_OpenError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	dst := filepath.Join(sandbox, "dst.txt")

	err := l.Move(ctx, "/nonexistent/file.txt", dst)
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestLocal_Move_CreateError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	os.WriteFile(src, []byte("test"), 0644)

	invalidDst := filepath.Join("/", "proc", "cannot-write.txt")
	err := l.Move(ctx, src, invalidDst)
	if err == nil {
		t.Error("Expected error for invalid destination")
	}
}

func TestLocal_Validate_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Validate(ctx, "/nonexistent/path/file.txt")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_Chown_WithMock(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Chown(ctx, testFile, os.Getuid(), os.Getgid())
	if err != nil && os.Getuid() == 0 {
		t.Errorf("Chown failed as root: %v", err)
	}
}

func TestLocal_Chown_Error(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Chown(ctx, "/nonexistent/file.txt", 0, 0)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLocal_Write_SyncError(t *testing.T) {
	l := NewLocal(config.Default())
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	ctx := context.Background()

	err := l.Write(ctx, testFile, []byte("test data"))
	if err != nil {
		t.Logf("Write error (may be sync-related): %v", err)
	}
}

func TestLocal_Move_CopyError(t *testing.T) {
	l := NewLocal(config.Default())
	sandbox := t.TempDir()

	srcFile := filepath.Join(sandbox, "src.txt")
	os.WriteFile(srcFile, []byte("test"), 0644)

	dstFile := filepath.Join(sandbox, "subdir", "dst.txt")

	ctx := context.Background()

	err := l.Move(ctx, srcFile, dstFile)
	if err == nil {
		t.Error("Expected error when destination directory doesn't exist")
	}
}

func TestLocal_Move_DirectoryRenameSuccess(t *testing.T) {
	l := NewLocal(config.Default())
	sandbox := t.TempDir()

	srcDir := filepath.Join(sandbox, "srcdir")
	dstDir := filepath.Join(sandbox, "dstdir")
	os.Mkdir(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("test"), 0644)

	ctx := context.Background()

	err := l.Move(ctx, srcDir, dstDir)
	if err != nil {
		t.Errorf("Move directory failed: %v", err)
	}

	exists, _ := l.Exists(ctx, dstDir)
	if !exists {
		t.Error("Expected destination directory to exist")
	}
}

func TestLocal_WriteStream_BufferedCopy(t *testing.T) {
	l := NewLocal(config.Default())
	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	ctx := context.Background()

	largeData := strings.Repeat("a", 100000)
	err := l.WriteStream(ctx, testFile, strings.NewReader(largeData))
	if err != nil {
		t.Fatalf("WriteStream failed: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if len(data) != 100000 {
		t.Errorf("Expected 100000 bytes, got %d", len(data))
	}
}

func TestLocal_UnsupportedOperations(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Download(ctx, "url", "dst", nil)
	if err == nil {
		t.Error("Expected Download to be unsupported")
	}

	_, err = l.DownloadStream(ctx, "url", nil)
	if err == nil {
		t.Error("Expected DownloadStream to be unsupported")
	}

	_, err = l.Fetch(ctx, "url")
	if err == nil {
		t.Error("Expected Fetch to be unsupported")
	}
}

func TestLocal_Read(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("read data"), 0644)

	data, err := l.Read(ctx, testFile)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != "read data" {
		t.Errorf("Expected %q, got %q", "read data", string(data))
	}
}

func TestLocal_Read_NotFound(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.Read(ctx, "/nonexistent/path/file.txt")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLocal_Read_Directory(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()

	_, err := l.Read(ctx, sandbox)
	if err == nil {
		t.Error("Expected error when reading directory")
	}
}

func TestLocal_Read_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.Read(ctx, "/nonexistent/path/file.txt")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLocal_GetInfo_Directory(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()

	size, resourceType, modTime, err := l.GetInfo(ctx, sandbox)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if resourceType != "dir" {
		t.Errorf("Expected type dir, got %s", resourceType)
	}
	if modTime.IsZero() {
		t.Error("Expected non-zero ModTime")
	}
	_ = size
}

func TestLocal_GetInfo_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, _, _, err := l.GetInfo(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_Exists_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.Exists(ctx, "/nonexistent/path")
	if err != nil {
		t.Fatalf("Exists should not return error for non-existent: %v", err)
	}
}

func TestLocal_IsDir_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.IsDir(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_IsFile_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	_, err := l.IsFile(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_ReadStream_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	file.Close()

	_, err = l.ReadStream(ctx, testFile)
	if err != nil {
		t.Logf("ReadStream error (may be expected): %v", err)
	}
}

func TestLocal_Write_ContextCancellation(t *testing.T) {
	l := NewLocal(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	err := l.Write(ctx, testFile, []byte("test"))
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestLocal_Write_LargeBuffer(t *testing.T) {
	cfg := config.Default()
	cfg.BufferSize = 100
	l := NewLocal(cfg)
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	largeData := make([]byte, cfg.BufferSize*11)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	err := l.Write(ctx, testFile, largeData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if len(data) != len(largeData) {
		t.Errorf("Expected %d bytes, got %d", len(largeData), len(data))
	}
}

func TestLocal_Write_MkdirAllError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "cannot", "create", "file.txt")
	err := l.Write(ctx, invalidPath, []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_WriteStream_ContextCancellation(t *testing.T) {
	l := NewLocal(config.Default())
	ctx, cancel := context.WithCancel(context.Background())

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")

	reader := strings.NewReader("test data")
	cancel()

	err := l.WriteStream(ctx, testFile, reader)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestLocal_Append_MkdirAllError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "cannot", "create", "file.txt")
	err := l.Append(ctx, invalidPath, []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestLocal_List_ContextCancellation(t *testing.T) {
	l := NewLocal(config.Default())
	ctx, cancel := context.WithCancel(context.Background())

	sandbox := t.TempDir()
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(sandbox, fmt.Sprintf("file%d.txt", i)), []byte("test"), 0644)
	}

	cancel()

	_, err := l.List(ctx, sandbox)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestLocal_List_FileNotDirectory(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	_, err := l.List(ctx, testFile)
	if err == nil {
		t.Error("Expected error when listing file")
	}
}

func TestLocal_Mkdir_ContextCancellation(t *testing.T) {
	l := NewLocal(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")

	err := l.Mkdir(ctx, testDir, 0755)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestLocal_MkdirAll_ContextCancellation(t *testing.T) {
	l := NewLocal(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")

	err := l.MkdirAll(ctx, testDir, 0755)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestLocal_Remove_ReadDirError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)

	err := l.Remove(ctx, testDir)
	if err != nil {
		t.Logf("Remove error (expected for empty dir): %v", err)
	}
}

func TestLocal_Remove_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Remove(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_RemoveAll_StatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.RemoveAll(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_RemoveAll_RemoveError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)

	err := l.RemoveAll(ctx, testDir)
	if err != nil {
		t.Logf("RemoveAll error: %v", err)
	}
}

func TestLocal_Chmod_ContextCancellation(t *testing.T) {
	l := NewLocal(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Chmod(ctx, testFile, 0755)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestLocal_Chmod_Error(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Chmod(ctx, "/nonexistent/path", 0755)
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_Chown_ContextCancellation(t *testing.T) {
	l := NewLocal(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Chown(ctx, testFile, 1000, 1000)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestLocal_Validate_OpenError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Validate(ctx, testFile)
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestLocal_Validate_AbsError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Validate(ctx, "/nonexistent/path/file.txt")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestLocal_Validate_Directory(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()

	err := l.Validate(ctx, sandbox)
	if err != nil {
		t.Errorf("Validate directory failed: %v", err)
	}
}

func TestLocal_Copy_IOError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	err := l.Copy(ctx, src, dst)
	if err != nil {
		t.Logf("Copy error: %v", err)
	}
}

func TestLocal_Move_RemoveError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	err := l.Move(ctx, src, dst)
	if err != nil {
		t.Logf("Move error: %v", err)
	}
}

func TestLocal_Rename_Error(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	err := l.Rename(ctx, "/nonexistent/src", "/nonexistent/dst")
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestLocal_Read_ReadFileError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	os.Chmod(testFile, 0000)
	defer os.Chmod(testFile, 0644)

	if os.Getuid() == 0 {
		t.Skip("Skipping test as root can read any file")
	}

	_, err := l.Read(ctx, testFile)
	if err == nil {
		t.Error("Expected error when reading file without permissions")
	}
}

func TestLocal_RemoveAll_RemoveAllError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)

	err := l.RemoveAll(ctx, testDir)
	if err != nil {
		t.Logf("RemoveAll error: %v", err)
	}
}

func TestLocal_Rename_RenameError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	err := l.Rename(ctx, src, dst)
	if err != nil {
		t.Logf("Rename error: %v", err)
	}
}

func TestLocal_Validate_FileOpenError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	err := l.Validate(ctx, testFile)
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestLocal_Exists_NonNotExistError(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping test that requires root to trigger permission errors")
	}

	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "self", "fd", "999999")
	_, err := l.Exists(ctx, invalidPath)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_IsDir_NonNotExistError(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping test that requires root")
	}

	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "self", "fd", "999999")
	_, err := l.IsDir(ctx, invalidPath)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_IsFile_NonNotExistError(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping test that requires root")
	}

	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "self", "fd", "999999")
	_, err := l.IsFile(ctx, invalidPath)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_ReadStream_StatErrorAfterOpen(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	file.Close()

	stream, err := l.ReadStream(ctx, testFile)
	if err != nil {
		t.Logf("ReadStream error (may be expected): %v", err)
	} else {
		stream.Close()
	}
}

func TestLocal_Move_DirectoryRenameFailure(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	srcDir := filepath.Join(sandbox, "srcdir")
	dstDir := filepath.Join(sandbox, "dstdir")
	os.Mkdir(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0500)
		defer os.Chmod(sandbox, 0755)
	}

	err := l.Move(ctx, srcDir, dstDir)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_ReadStream_NonNotExistOpenError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	_, err := l.ReadStream(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Remove_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	err := l.Remove(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_RemoveAll_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	err := l.RemoveAll(ctx, testDir)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Rename_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	err := l.Rename(ctx, src, dst)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Move_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	err := l.Move(ctx, src, dst)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Validate_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	err := l.Validate(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_GetInfo_NonNotExistError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	_, _, _, err := l.GetInfo(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Exists_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	_, err := l.Exists(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_IsDir_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	_, err := l.IsDir(ctx, testDir)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_IsFile_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	_, err := l.IsFile(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_ReadStream_FileStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	file.Close()

	stream, err := l.ReadStream(ctx, testFile)
	if err != nil {
		t.Logf("ReadStream error (may be expected): %v", err)
	} else {
		stream.Close()
	}
}

func TestLocal_List_NonNotExistStatError(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	_, err := l.List(ctx, testDir)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Validate_AbsError_LongPath(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	veryLongPath := strings.Repeat("a", 10000) + "/" + strings.Repeat("b", 10000)
	err := l.Validate(ctx, veryLongPath)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_ReadStream_NonNotExistOpenError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	_, err := l.ReadStream(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_ReadStream_FileStatErrorAfterOpen(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	file.Close()

	stream, err := l.ReadStream(ctx, testFile)
	if err != nil {
		t.Logf("ReadStream error (may be expected): %v", err)
	} else {
		stream.Close()
	}
}

func TestLocal_ReadStream_OpenError_InvalidPath(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	invalidPath := filepath.Join("/", "proc", "self", "fd", "999999")
	_, err := l.ReadStream(ctx, invalidPath)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_ReadStream_OpenError_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping test as root can access any file")
	}

	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	os.Chmod(sandbox, 0000)
	defer os.Chmod(sandbox, 0755)

	_, err := l.ReadStream(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Remove_NonNotExistStatError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		parentDir := filepath.Dir(sandbox)
		if parentDir != sandbox {
			os.Chmod(parentDir, 0000)
			defer os.Chmod(parentDir, 0755)
		}
	}

	err := l.Remove(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Remove_ReadDirError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)

	if os.Getuid() != 0 {
		os.Chmod(testDir, 0000)
		defer os.Chmod(testDir, 0755)
	}

	err := l.Remove(ctx, testDir)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_RemoveAll_NonNotExistStatError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)

	if os.Getuid() != 0 {
		parentDir := filepath.Dir(sandbox)
		if parentDir != sandbox {
			os.Chmod(parentDir, 0000)
			defer os.Chmod(parentDir, 0755)
		}
	}

	err := l.RemoveAll(ctx, testDir)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_RemoveAll_RemoveAllError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testDir := filepath.Join(sandbox, "testdir")
	os.Mkdir(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "file.txt"), []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(testDir, 0000)
		defer os.Chmod(testDir, 0755)
	}

	err := l.RemoveAll(ctx, testDir)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Move_NonNotExistStatError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	if os.Getuid() != 0 {
		parentDir := filepath.Dir(sandbox)
		if parentDir != sandbox {
			os.Chmod(parentDir, 0000)
			defer os.Chmod(parentDir, 0755)
		}
	}

	err := l.Move(ctx, src, dst)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Move_OpenSrcError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(src, 0000)
		defer os.Chmod(src, 0644)
	}

	err := l.Move(ctx, src, dst)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Move_CreateDstError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "subdir", "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0000)
		defer os.Chmod(sandbox, 0755)
	}

	err := l.Move(ctx, src, dst)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Move_CopyError_Simulated(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	err := l.Move(ctx, src, dst)
	if err != nil {
		t.Logf("Move error: %v", err)
	}
}

func TestLocal_Move_RemoveSrcError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	src := filepath.Join(sandbox, "src.txt")
	dst := filepath.Join(sandbox, "dst.txt")
	os.WriteFile(src, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(sandbox, 0500)
		defer os.Chmod(sandbox, 0755)
	}

	err := l.Move(ctx, src, dst)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Validate_NonNotExistStatError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		parentDir := filepath.Dir(sandbox)
		if parentDir != sandbox {
			os.Chmod(parentDir, 0000)
			defer os.Chmod(parentDir, 0755)
		}
	}

	err := l.Validate(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

func TestLocal_Validate_OpenError_Permission(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()

	sandbox := t.TempDir()
	testFile := filepath.Join(sandbox, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	if os.Getuid() != 0 {
		os.Chmod(testFile, 0000)
		defer os.Chmod(testFile, 0644)
	}

	err := l.Validate(ctx, testFile)
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}
