package fetchnshare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStrategy handles local filesystem operations
type LocalStrategy struct{}

func (l *LocalStrategy) GetInfo(ctx context.Context, path string) (*ResourceInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var info = &ResourceInfo{}
	info.Size = stat.Size()
	info.ModTime = stat.ModTime()
	if stat.IsDir() {
		info.Type = ResourceType("dir")
	} else {
		info.Type = ResourceType("file")
	}
	return info, nil
}

func (l *LocalStrategy) Exists(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (l *LocalStrategy) IsDir(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if info.IsDir() {
		return true, nil
	} else {
		return false, nil
	}
}

func (l *LocalStrategy) IsFile(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if info.IsDir() {
		return false, nil
	} else {
		return true, nil
	}
}

func (l *LocalStrategy) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open local file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat local file: %w", err)
	}

	if info.IsDir() {
		file.Close()
		return nil, fmt.Errorf("path is a directory, not a file")
	}

	return file, nil
}

func (l *LocalStrategy) Write(ctx context.Context, path string, data []byte) error {

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// If data is too large, use WriteStream
	if len(data) > Maxsize {
		return l.WriteStream(ctx, path, bytes.NewReader(data))
	} else {

		// Write data to file
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
	}

	return nil
}

func (l *LocalStrategy) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Create or truncate the file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	// Copy data from reader to file
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(file, reader)
		done <- err
	}()

	select {
	case <-ctx.Done():
		file.Close()
		os.Remove(path) // Clean up partial file
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to write data: %w", err)
		}
	}
	return nil
}

func (l *LocalStrategy) Append(ctx context.Context, path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Open file in append mode, create if not exists
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file for appending: %w", err)
	}
	defer file.Close()

	// Write data to file
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to append data: %w", err)
	}

	return nil
}

func (l *LocalStrategy) List(ctx context.Context, path string) ([]ResourceInfo, error) {
	// Check if directory exists
	entries, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if !entries.IsDir() {
		return nil, fmt.Errorf("path is not a directory")
	}

	// Read directory entries
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var resources []ResourceInfo

	for _, entry := range dirEntries {
		// We gotta check just in case the context was cancelled during processing
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		entryPath := filepath.Join(path, entry.Name())
		info, err := l.GetInfo(ctx, entryPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get info for %s: %w", entryPath, err)
		}

		resources = append(resources, *info)
	}

	return resources, nil
}

func (l *LocalStrategy) Mkdir(ctx context.Context, path string, perm os.FileMode) error {

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Create the directory, fails if parent dirs don't exist
	if err := os.Mkdir(path, perm); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	} else {
		return nil
	}
}

func (l *LocalStrategy) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Create the directory and all necessary parents
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	return nil
}

func (l *LocalStrategy) Remove(ctx context.Context, path string) error {

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	} else if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	// Handle directories and files differently
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Errorf("failed to read directory: %w", err)
		}

		if len(entries) > 0 {
			return fmt.Errorf("directory not empty: %s", path)
		}

		// Directory is empty → safe to remove
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove directory: %w", err)
		}
	} else {
		// Regular file
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove file: %w", err)
		}
	}

	return nil
}

func (l *LocalStrategy) RemoveAll(ctx context.Context, path string) error {

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	} else if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	// Remove file or directory (recursively if directory)
	if info.IsDir() {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}

	if err != nil {
		return fmt.Errorf("failed to remove %s: %w", path, err)
	}

	return nil
}

func (l *LocalStrategy) Copy(ctx context.Context, src, dst string) error {
	var srcReader io.ReadCloser
	var err error

	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	srcReader = f
	defer f.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	// Copy data with streaming (no full buffering)
	_, err = io.Copy(dstFile, srcReader)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	return nil
}

func (l *LocalStrategy) Move(ctx context.Context, src, dst string) error {

	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return fmt.Errorf("source does not exist: %s", src)
	} else if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}

	if info.IsDir() {
		if err := os.Rename(src, dst); err == nil {
			return nil
		}
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	if err := srcFile.Close(); err != nil {
		return fmt.Errorf("failed to close source file: %w", err)
	}

	// Remove the source only after successful copy
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("failed to remove original file: %w", err)
	}

	return nil
}

func (l *LocalStrategy) Rename(ctx context.Context, src, dst string) error {
	_, err := os.Stat(src)
	if os.IsNotExist(err) {
		return fmt.Errorf("source does not exist: %s", src)
	} else if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}
	err = os.Rename(src, dst)
	if err == nil {
		return nil
	}

	return fmt.Errorf("failed to rename file: %w", err)
}

func (l *LocalStrategy) Chmod(ctx context.Context, path string, mode os.FileMode) error {

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := os.Chmod(path, mode)
	if err != nil {
		return fmt.Errorf("failed to change file mode: %w", err)
	}
	return nil
}

func (l *LocalStrategy) Chown(ctx context.Context, path string, uid, gid int) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := os.Chown(path, uid, gid)
	if err != nil {
		return fmt.Errorf("failed to change file ownership: %w", err)
	}

	return nil
}

func (l *LocalStrategy) Download(ctx context.Context, url, dst string, progress func(int)) error {
	return fmt.Errorf("Download not supported from local files")
}

func (l *LocalStrategy) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	return nil, fmt.Errorf("DownloadStream not supported from local files")
}

func (l *LocalStrategy) Fetch(ctx context.Context, url string) ([]byte, error) {
	return nil, fmt.Errorf("Fetch not supported from local files")
}

func (l *LocalStrategy) Validate(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", abs)
	} else if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		file, err := os.Open(abs)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		file.Close()
	}

	return nil
}
