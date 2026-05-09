package strategies

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/fns/errors"
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
	cleanPath := filepath.Clean(path)
	stat, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "", time.Time{}, errors.NotFound("GetInfo", cleanPath)
		}
		return 0, "", time.Time{}, errors.Op("GetInfo", cleanPath, err)
	}

	resourceType := "file"
	if stat.IsDir() {
		resourceType = "dir"
	}

	return stat.Size(), resourceType, stat.ModTime(), nil
}

func (l *Local) Exists(ctx context.Context, path string) (bool, error) {
	cleanPath := filepath.Clean(path)
	_, err := os.Stat(cleanPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, errors.Op("Exists", cleanPath, err)
}

func (l *Local) IsDir(ctx context.Context, path string) (bool, error) {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, errors.NotFound("IsDir", cleanPath)
		}
		return false, errors.Op("IsDir", cleanPath, err)
	}

	return info.IsDir(), nil
}

func (l *Local) IsFile(ctx context.Context, path string) (bool, error) {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, errors.NotFound("IsFile", cleanPath)
		}
		return false, errors.Op("IsFile", cleanPath, err)
	}

	return !info.IsDir(), nil
}

func (l *Local) Read(ctx context.Context, path string) ([]byte, error) {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFound("Read", cleanPath)
		}
		return nil, errors.Op("Read", cleanPath, err)
	}

	if info.IsDir() {
		return nil, errors.Op("Read", cleanPath, fmt.Errorf("path is a directory"))
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, errors.Op("Read", cleanPath, err)
	}

	return data, nil
}

func (l *Local) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	cleanPath := filepath.Clean(path)
	file, err := os.Open(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFound("ReadStream", cleanPath)
		}
		return nil, errors.Op("ReadStream", cleanPath, err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, errors.Op("ReadStream", cleanPath, err)
	}

	if info.IsDir() {
		_ = file.Close()
		return nil, errors.Op("ReadStream", cleanPath, fmt.Errorf("path is a directory"))
	}

	return file, nil
}

func (l *Local) Write(ctx context.Context, path string, data []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return errors.Op("Write", cleanPath, fmt.Errorf("failed to create directories: %w", err))
	}

	if len(data) > l.bufferSize*10 {
		return l.WriteStream(ctx, cleanPath, bytes.NewReader(data))
	}

	if err := os.WriteFile(cleanPath, data, 0o600); err != nil {
		return errors.Op("Write", cleanPath, err)
	}

	return nil
}

func (l *Local) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return errors.Op("WriteStream", cleanPath, fmt.Errorf("failed to create directories: %w", err))
	}

	file, err := os.Create(cleanPath)
	if err != nil {
		return errors.Op("WriteStream", cleanPath, err)
	}
	defer file.Close() //nolint:errcheck

	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(file, reader)
		done <- err
	}()

	select {
	case <-ctx.Done():
		_ = file.Close()
		_ = os.Remove(cleanPath)
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return errors.Op("WriteStream", cleanPath, err)
		}
	}

	return nil
}

func (l *Local) Append(ctx context.Context, path string, data []byte) error {
	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return errors.Op("Append", cleanPath, fmt.Errorf("failed to create directories: %w", err))
	}

	file, err := os.OpenFile(cleanPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.Op("Append", cleanPath, err)
	}
	defer file.Close() //nolint:errcheck

	if _, err := file.Write(data); err != nil {
		return errors.Op("Append", cleanPath, err)
	}

	return nil
}

func (l *Local) List(ctx context.Context, path string) ([]string, error) {
	cleanPath := filepath.Clean(path)
	entries, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFound("List", cleanPath)
		}
		return nil, errors.Op("List", cleanPath, err)
	}

	if !entries.IsDir() {
		return nil, errors.Op("List", cleanPath, fmt.Errorf("path is not a directory"))
	}

	dirEntries, err := os.ReadDir(cleanPath)
	if err != nil {
		return nil, errors.Op("List", cleanPath, err)
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

	cleanPath := filepath.Clean(path)
	if err := os.Mkdir(cleanPath, perm); err != nil {
		return errors.Op("Mkdir", cleanPath, err)
	}
	return nil
}

func (l *Local) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	cleanPath := filepath.Clean(path)
	if err := os.MkdirAll(cleanPath, perm); err != nil {
		return errors.Op("MkdirAll", cleanPath, err)
	}
	return nil
}

func (l *Local) Remove(ctx context.Context, path string) error {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if os.IsNotExist(err) {
		return errors.NotFound("Remove", cleanPath)
	} else if err != nil {
		return errors.Op("Remove", cleanPath, err)
	}

	if info.IsDir() { //nolint:nestif
		entries, err := os.ReadDir(cleanPath)
		if err != nil {
			return errors.Op("Remove", cleanPath, err)
		}

		if len(entries) > 0 {
			return errors.Op("Remove", cleanPath, fmt.Errorf("directory not empty"))
		}
	}

	if err := os.Remove(cleanPath); err != nil {
		return errors.Op("Remove", cleanPath, err)
	}

	return nil
}

func (l *Local) RemoveAll(ctx context.Context, path string) error {
	cleanPath := filepath.Clean(path)
	_, err := os.Stat(cleanPath)
	if os.IsNotExist(err) {
		return errors.NotFound("RemoveAll", cleanPath)
	} else if err != nil {
		return errors.Op("RemoveAll", cleanPath, err)
	}

	if err := os.RemoveAll(cleanPath); err != nil {
		return errors.Op("RemoveAll", cleanPath, err)
	}

	return nil
}

func (l *Local) Copy(ctx context.Context, src, dst string) error {
	cleanSrc := filepath.Clean(src)
	cleanDst := filepath.Clean(dst)
	srcFile, err := os.Open(cleanSrc)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.NotFound("Copy", cleanSrc)
		}
		return errors.Op("Copy", cleanSrc, err)
	}
	defer srcFile.Close() //nolint:errcheck

	dstFile, err := os.Create(cleanDst)
	if err != nil {
		return errors.Op("Copy", cleanDst, err)
	}
	defer dstFile.Close() //nolint:errcheck

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return errors.Op("Copy", cleanDst, err)
	}

	return nil
}

func (l *Local) Move(ctx context.Context, src, dst string) error {
	cleanSrc := filepath.Clean(src)
	cleanDst := filepath.Clean(dst)
	info, err := os.Stat(cleanSrc)
	if os.IsNotExist(err) {
		return errors.NotFound("Move", cleanSrc)
	} else if err != nil {
		return errors.Op("Move", cleanSrc, err)
	}

	if info.IsDir() {
		if err := os.Rename(cleanSrc, cleanDst); err == nil {
			return nil
		}
	}

	srcFile, err := os.Open(cleanSrc)
	if err != nil {
		return errors.Op("Move", cleanSrc, err)
	}
	defer srcFile.Close() //nolint:errcheck

	dstFile, err := os.Create(cleanDst)
	if err != nil {
		return errors.Op("Move", cleanDst, err)
	}
	defer dstFile.Close() //nolint:errcheck

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return errors.Op("Move", cleanDst, err)
	}

	_ = srcFile.Close()

	if err := os.Remove(cleanSrc); err != nil {
		return errors.Op("Move", cleanSrc, fmt.Errorf("failed to remove source: %w", err))
	}

	return nil
}

func (l *Local) Rename(ctx context.Context, src, dst string) error {
	cleanSrc := filepath.Clean(src)
	cleanDst := filepath.Clean(dst)
	_, err := os.Stat(cleanSrc)
	if os.IsNotExist(err) {
		return errors.NotFound("Rename", cleanSrc)
	} else if err != nil {
		return errors.Op("Rename", cleanSrc, err)
	}

	if err := os.Rename(cleanSrc, cleanDst); err != nil {
		return errors.Op("Rename", cleanSrc, err)
	}

	return nil
}

func (l *Local) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	cleanPath := filepath.Clean(path)
	if err := os.Chmod(cleanPath, mode); err != nil {
		return errors.Op("Chmod", cleanPath, err)
	}
	return nil
}

func (l *Local) Chown(ctx context.Context, path string, uid, gid int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	cleanPath := filepath.Clean(path)
	if err := os.Chown(cleanPath, uid, gid); err != nil {
		return errors.Op("Chown", cleanPath, err)
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
	cleanPath := filepath.Clean(path)
	abs, err := filepath.Abs(cleanPath)
	if err != nil {
		return errors.Op("Validate", cleanPath, err)
	}

	abs = filepath.Clean(abs)
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
		_ = file.Close()
	}

	return nil
}
