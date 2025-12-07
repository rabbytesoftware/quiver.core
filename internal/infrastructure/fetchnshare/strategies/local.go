package strategies

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/config"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/errors"
)

type Local struct {
	bufferSize int
}

func NewLocal(cfg config.Config) *Local {
	return &Local{
		bufferSize: cfg.BufferSize,
	}
}

func (l *Local) GetInfo(ctx context.Context, path string) (int64, string, time.Time, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "", time.Time{}, errors.NotFound("GetInfo", path)
		}
		return 0, "", time.Time{}, errors.Op("GetInfo", path, err)
	}

	resourceType := "file"
	if stat.IsDir() {
		resourceType = "dir"
	}

	return stat.Size(), resourceType, stat.ModTime(), nil
}

func (l *Local) Exists(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, errors.Op("Exists", path, err)
}

func (l *Local) IsDir(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, errors.NotFound("IsDir", path)
		}
		return false, errors.Op("IsDir", path, err)
	}

	return info.IsDir(), nil
}

func (l *Local) IsFile(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, errors.NotFound("IsFile", path)
		}
		return false, errors.Op("IsFile", path, err)
	}

	return !info.IsDir(), nil
}

func (l *Local) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFound("ReadStream", path)
		}
		return nil, errors.Op("ReadStream", path, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, errors.Op("ReadStream", path, err)
	}

	if info.IsDir() {
		file.Close()
		return nil, errors.Op("ReadStream", path, fmt.Errorf("path is a directory"))
	}

	return file, nil
}

func (l *Local) Write(ctx context.Context, path string, data []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if path == "" || strings.ContainsRune(path, 0) || strings.HasSuffix(path, string(os.PathSeparator)) {
		return errors.Op("Write", path, fmt.Errorf("invalid file path"))
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Op("Write", path, fmt.Errorf("failed to create directories: %w", err))
	}

	if len(data) > l.bufferSize*10 {
		return l.WriteStream(ctx, path, bytes.NewReader(data))
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return errors.Op("Write", path, err)
	}

	return nil
}

func (l *Local) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	if path == "" || strings.ContainsRune(path, 0) || strings.HasSuffix(path, string(os.PathSeparator)) {
		return errors.Op("WriteStream", path, fmt.Errorf("invalid file path"))
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Op("WriteStream", path, fmt.Errorf("failed to create directories: %w", err))
	}

	file, err := os.Create(path)
	if err != nil {
		return errors.Op("WriteStream", path, err)
	}
	defer file.Close()

	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(file, reader)
		done <- err
	}()

	select {
	case <-ctx.Done():
		file.Close()
		os.Remove(path)
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return errors.Op("WriteStream", path, err)
		}
	}

	return nil
}

func (l *Local) Append(ctx context.Context, path string, data []byte) error {
	if path == "" || strings.ContainsRune(path, 0) || strings.HasSuffix(path, string(os.PathSeparator)) {
		return errors.Op("Append", path, fmt.Errorf("invalid file path"))
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Op("Append", path, fmt.Errorf("failed to create directories: %w", err))
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Op("Append", path, err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return errors.Op("Append", path, err)
	}

	return nil
}

func (l *Local) List(ctx context.Context, path string) ([]string, error) {
	entries, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFound("List", path)
		}
		return nil, errors.Op("List", path, err)
	}

	if !entries.IsDir() {
		return nil, errors.Op("List", path, fmt.Errorf("path is not a directory"))
	}

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, errors.Op("List", path, err)
	}

	var names []string
	for _, entry := range dirEntries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		names = append(names, entry.Name())
	}

	return names, nil
}

func (l *Local) Mkdir(ctx context.Context, path string, perm os.FileMode) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := os.Mkdir(path, perm); err != nil {
		return errors.Op("Mkdir", path, err)
	}
	return nil
}

func (l *Local) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if path == "" || strings.ContainsRune(path, 0) || strings.HasSuffix(path, string(os.PathSeparator)) {
		return errors.Op("MkdirAll", path, fmt.Errorf("invalid file path"))
	}

	if err := os.MkdirAll(path, perm); err != nil {
		return errors.Op("MkdirAll", path, err)
	}
	return nil
}

func (l *Local) Remove(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return errors.NotFound("Remove", path)
	} else if err != nil {
		return errors.Op("Remove", path, err)
	}

	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return errors.Op("Remove", path, err)
		}

		if len(entries) > 0 {
			return errors.Op("Remove", path, fmt.Errorf("directory not empty"))
		}
	}

	if err := os.Remove(path); err != nil {
		return errors.Op("Remove", path, err)
	}

	return nil
}

func (l *Local) RemoveAll(ctx context.Context, path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return errors.NotFound("RemoveAll", path)
	} else if err != nil {
		return errors.Op("RemoveAll", path, err)
	}

	if err := os.RemoveAll(path); err != nil {
		return errors.Op("RemoveAll", path, err)
	}

	return nil
}

func (l *Local) Copy(ctx context.Context, src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.NotFound("Copy", src)
		}
		return errors.Op("Copy", src, err)
	}
	defer srcFile.Close()

	if dst == "" || strings.ContainsRune(dst, 0) || strings.HasSuffix(dst, string(os.PathSeparator)) {
		return errors.Op("Copy", dst, fmt.Errorf("invalid file path"))
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return errors.Op("Copy", dst, err)
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return errors.Op("Copy", dst, err)
	}

	return nil
}

func (l *Local) Move(ctx context.Context, src, dst string) error {
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return errors.NotFound("Move", src)
	} else if err != nil {
		return errors.Op("Move", src, err)
	}

	if dst == "" || strings.ContainsRune(dst, 0) || strings.HasSuffix(dst, string(os.PathSeparator)) {
		return errors.Op("Move", dst, fmt.Errorf("invalid file path"))
	}

	if info.IsDir() {
		if err := os.Rename(src, dst); err == nil {
			return nil
		}
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return errors.Op("Move", src, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return errors.Op("Move", dst, err)
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return errors.Op("Move", dst, err)
	}

	srcFile.Close()

	if err := os.Remove(src); err != nil {
		return errors.Op("Move", src, fmt.Errorf("failed to remove source: %w", err))
	}

	return nil
}

func (l *Local) Rename(ctx context.Context, src, dst string) error {
	_, err := os.Stat(src)
	if os.IsNotExist(err) {
		return errors.NotFound("Rename", src)
	} else if err != nil {
		return errors.Op("Rename", src, err)
	}

	if err := os.Rename(src, dst); err != nil {
		return errors.Op("Rename", src, err)
	}

	return nil
}

func (l *Local) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := os.Chmod(path, mode); err != nil {
		return errors.Op("Chmod", path, err)
	}
	return nil
}

func (l *Local) Chown(ctx context.Context, path string, uid, gid int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := os.Chown(path, uid, gid); err != nil {
		return errors.Op("Chown", path, err)
	}

	return nil
}

func (l *Local) Download(ctx context.Context, url, dst string, progress func(int)) error {
	return errors.Unsupported("Download", "local files")
}

func (l *Local) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	return nil, errors.Unsupported("DownloadStream", "local files")
}

func (l *Local) Fetch(ctx context.Context, url string) ([]byte, error) {
	return nil, errors.Unsupported("Fetch", "local files")
}

func (l *Local) Validate(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return errors.Op("Validate", path, err)
	}

	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return errors.NotFound("Validate", abs)
	} else if err != nil {
		return errors.Op("Validate", abs, err)
	}

	if !info.IsDir() {
		file, err := os.Open(abs)
		if err != nil {
			return errors.Op("Validate", abs, err)
		}
		file.Close()
	}

	return nil
}
