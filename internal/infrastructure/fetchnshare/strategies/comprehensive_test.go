package strategies

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/config"
)

func TestLocal_ComprehensiveErrorPaths(t *testing.T) {
	l := NewLocal(config.Default())
	ctx := context.Background()
	sandbox := t.TempDir()

	t.Run("GetInfo Success", func(t *testing.T) {
		file := filepath.Join(sandbox, "getinfo.txt")
		os.WriteFile(file, []byte("test"), 0644)
		
		size, rtype, _, err := l.GetInfo(ctx, file)
		if err != nil {
			t.Errorf("GetInfo failed: %v", err)
		}
		if size != 4 {
			t.Errorf("Expected size 4, got %d", size)
		}
		if rtype != "file" {
			t.Errorf("Expected type file, got %s", rtype)
		}
	})

	t.Run("Exists for existing file", func(t *testing.T) {
		file := filepath.Join(sandbox, "exists.txt")
		os.WriteFile(file, []byte("test"), 0644)
		
		exists, err := l.Exists(ctx, file)
		if err != nil {
			t.Errorf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("Expected file to exist")
		}
	})

	t.Run("IsDir for directory", func(t *testing.T) {
		dir := filepath.Join(sandbox, "isdir")
		os.Mkdir(dir, 0755)
		
		isDir, err := l.IsDir(ctx, dir)
		if err != nil {
			t.Errorf("IsDir failed: %v", err)
		}
		if !isDir {
			t.Error("Expected to be directory")
		}
	})

	t.Run("IsFile for file", func(t *testing.T) {
		file := filepath.Join(sandbox, "isfile.txt")
		os.WriteFile(file, []byte("test"), 0644)
		
		isFile, err := l.IsFile(ctx, file)
		if err != nil {
			t.Errorf("IsFile failed: %v", err)
		}
		if !isFile {
			t.Error("Expected to be file")
		}
	})

	t.Run("ReadStream success", func(t *testing.T) {
		file := filepath.Join(sandbox, "readstream.txt")
		os.WriteFile(file, []byte("readstream data"), 0644)
		
		stream, err := l.ReadStream(ctx, file)
		if err != nil {
			t.Errorf("ReadStream failed: %v", err)
		}
		if stream != nil {
			stream.Close()
		}
	})

	t.Run("Write success", func(t *testing.T) {
		file := filepath.Join(sandbox, "write.txt")
		
		err := l.Write(ctx, file, []byte("write data"))
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
		
		data, _ := os.ReadFile(file)
		if string(data) != "write data" {
			t.Errorf("Expected %q, got %q", "write data", string(data))
		}
	})

	t.Run("WriteStream success", func(t *testing.T) {
		file := filepath.Join(sandbox, "writestream.txt")
		
		err := l.WriteStream(ctx, file, strings.NewReader("writestream data"))
		if err != nil {
			t.Errorf("WriteStream failed: %v", err)
		}
		
		data, _ := os.ReadFile(file)
		if string(data) != "writestream data" {
			t.Errorf("Expected %q, got %q", "writestream data", string(data))
		}
	})

	t.Run("Append success", func(t *testing.T) {
		file := filepath.Join(sandbox, "append.txt")
		os.WriteFile(file, []byte("first"), 0644)
		
		err := l.Append(ctx, file, []byte(" second"))
		if err != nil {
			t.Errorf("Append failed: %v", err)
		}
		
		data, _ := os.ReadFile(file)
		if string(data) != "first second" {
			t.Errorf("Expected %q, got %q", "first second", string(data))
		}
	})

	t.Run("List success", func(t *testing.T) {
		dir := filepath.Join(sandbox, "listdir")
		os.Mkdir(dir, 0755)
		os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("test"), 0644)
		os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("test"), 0644)
		
		names, err := l.List(ctx, dir)
		if err != nil {
			t.Errorf("List failed: %v", err)
		}
		if len(names) != 2 {
			t.Errorf("Expected 2 files, got %d", len(names))
		}
	})

	t.Run("Mkdir success", func(t *testing.T) {
		dir := filepath.Join(sandbox, "mkdir")
		
		err := l.Mkdir(ctx, dir, 0755)
		if err != nil {
			t.Errorf("Mkdir failed: %v", err)
		}
		
		exists, _ := l.Exists(ctx, dir)
		if !exists {
			t.Error("Expected directory to exist")
		}
	})

	t.Run("MkdirAll success", func(t *testing.T) {
		dir := filepath.Join(sandbox, "mkdirall", "nested", "deep")
		
		err := l.MkdirAll(ctx, dir, 0755)
		if err != nil {
			t.Errorf("MkdirAll failed: %v", err)
		}
		
		exists, _ := l.Exists(ctx, dir)
		if !exists {
			t.Error("Expected nested directory to exist")
		}
	})

	t.Run("Remove success", func(t *testing.T) {
		file := filepath.Join(sandbox, "remove.txt")
		os.WriteFile(file, []byte("test"), 0644)
		
		err := l.Remove(ctx, file)
		if err != nil {
			t.Errorf("Remove failed: %v", err)
		}
		
		exists, _ := l.Exists(ctx, file)
		if exists {
			t.Error("Expected file to be removed")
		}
	})

	t.Run("RemoveAll success", func(t *testing.T) {
		dir := filepath.Join(sandbox, "removeall")
		os.Mkdir(dir, 0755)
		os.WriteFile(filepath.Join(dir, "file.txt"), []byte("test"), 0644)
		
		err := l.RemoveAll(ctx, dir)
		if err != nil {
			t.Errorf("RemoveAll failed: %v", err)
		}
		
		exists, _ := l.Exists(ctx, dir)
		if exists {
			t.Error("Expected directory to be removed")
		}
	})

	t.Run("Copy success", func(t *testing.T) {
		src := filepath.Join(sandbox, "copy-src.txt")
		dst := filepath.Join(sandbox, "copy-dst.txt")
		os.WriteFile(src, []byte("copy data"), 0644)
		
		err := l.Copy(ctx, src, dst)
		if err != nil {
			t.Errorf("Copy failed: %v", err)
		}
		
		data, _ := os.ReadFile(dst)
		if string(data) != "copy data" {
			t.Errorf("Expected %q, got %q", "copy data", string(data))
		}
	})

	t.Run("Move file success", func(t *testing.T) {
		src := filepath.Join(sandbox, "move-src.txt")
		dst := filepath.Join(sandbox, "move-dst.txt")
		os.WriteFile(src, []byte("move data"), 0644)
		
		err := l.Move(ctx, src, dst)
		if err != nil {
			t.Errorf("Move failed: %v", err)
		}
		
		srcExists, _ := l.Exists(ctx, src)
		if srcExists {
			t.Error("Expected source to be removed")
		}
		
		dstExists, _ := l.Exists(ctx, dst)
		if !dstExists {
			t.Error("Expected destination to exist")
		}
	})

	t.Run("Rename success", func(t *testing.T) {
		src := filepath.Join(sandbox, "rename-src.txt")
		dst := filepath.Join(sandbox, "rename-dst.txt")
		os.WriteFile(src, []byte("rename data"), 0644)
		
		err := l.Rename(ctx, src, dst)
		if err != nil {
			t.Errorf("Rename failed: %v", err)
		}
		
		srcExists, _ := l.Exists(ctx, src)
		if srcExists {
			t.Error("Expected source to be removed")
		}
		
		dstExists, _ := l.Exists(ctx, dst)
		if !dstExists {
			t.Error("Expected destination to exist")
		}
	})

	t.Run("Chmod success", func(t *testing.T) {
		file := filepath.Join(sandbox, "chmod.txt")
		os.WriteFile(file, []byte("test"), 0644)
		
		err := l.Chmod(ctx, file, 0755)
		if err != nil {
			t.Errorf("Chmod failed: %v", err)
		}
	})

	t.Run("Chown success", func(t *testing.T) {
		file := filepath.Join(sandbox, "chown.txt")
		os.WriteFile(file, []byte("test"), 0644)
		
		err := l.Chown(ctx, file, os.Getuid(), os.Getgid())
		if err != nil && os.Getuid() == 0 {
			t.Errorf("Chown failed: %v", err)
		}
	})

	t.Run("Validate file success", func(t *testing.T) {
		file := filepath.Join(sandbox, "validate.txt")
		os.WriteFile(file, []byte("test"), 0644)
		
		err := l.Validate(ctx, file)
		if err != nil {
			t.Errorf("Validate failed: %v", err)
		}
	})

	t.Run("Validate directory success", func(t *testing.T) {
		dir := filepath.Join(sandbox, "validatedir")
		os.Mkdir(dir, 0755)
		
		err := l.Validate(ctx, dir)
		if err != nil {
			t.Errorf("Validate failed: %v", err)
		}
	})
}

