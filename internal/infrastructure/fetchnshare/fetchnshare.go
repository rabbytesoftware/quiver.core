package fetchnshare

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const Maxsize = 20 * 1024 * 1024 // 20MB PLACEHOLDER for max size to read into memory

// interface for remote and local strategies
type ResourceStrategy interface {
	GetInfo(ctx context.Context, path string) (*ResourceInfo, error)
	Exists(ctx context.Context, path string) (bool, error)
	IsDir(ctx context.Context, path string) (bool, error)
	IsFile(ctx context.Context, path string) (bool, error)
	ReadStream(ctx context.Context, path string) (io.ReadCloser, error)
	Write(ctx context.Context, path string, data []byte) error
	WriteStream(ctx context.Context, path string, reader io.Reader) error
	Append(ctx context.Context, path string, data []byte) error
	List(ctx context.Context, path string) ([]ResourceInfo, error)
	Mkdir(ctx context.Context, path string, perm os.FileMode) error
	MkdirAll(ctx context.Context, path string, perm os.FileMode) error
	Remove(ctx context.Context, path string) error
	RemoveAll(ctx context.Context, path string) error
	Copy(ctx context.Context, src, dst string) error
	Move(ctx context.Context, src, dst string) error
	Rename(ctx context.Context, src, dst string) error
	Chmod(ctx context.Context, path string, mode os.FileMode) error
	Chown(ctx context.Context, path string, uid, gid int) error
	Download(ctx context.Context, url, dst string, progress func(int)) error
	DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error)
	Fetch(ctx context.Context, url string) ([]byte, error)
	Validate(ctx context.Context, path string) error
}

type FNS struct {
	localStrat  ResourceStrategy
	remoteStrat ResourceStrategy
	cache       *Cache
}

func NewFNS() FNSInterface {
	return &FNS{
		localStrat:  &LocalStrategy{},
		remoteStrat: &RemoteStrategy{},
	}
}

// stratFor selects the appropriate strategy based on the path type (local or remote).
func (f *FNS) stratFor(path string) ResourceStrategy {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return f.remoteStrat
	}
	return f.localStrat
}

// GetInfo retrieves metadata information about a resource (file or directory).
// It returns ResourceInfo containing size, permissions, modification time, and other attributes.
// Supports both local filesystem paths and remote URLs (HTTP/HTTPS).
func (f *FNS) GetInfo(ctx context.Context, path string) (*ResourceInfo, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return f.stratFor(path).GetInfo(ctx, path)
}

// Exists checks whether a resource exists at the given path.
// Returns true if the resource exists, false otherwise.
// Works with both local filesystem paths and remote URLs.
func (f *FNS) Exists(ctx context.Context, path string) (bool, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return false, err
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	return f.stratFor(path).Exists(ctx, path)
}

// IsDir checks whether the resource at the given path is a directory.
// Returns true if the resource is a directory, false if it's a file or doesn't exist.
// Only works with local filesystem paths.
func (f *FNS) IsDir(ctx context.Context, path string) (bool, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return false, err
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	return f.stratFor(path).IsDir(ctx, path)
}

// IsFile checks whether the resource at the given path is a regular file.
// Returns true if the resource is a file, false if it's a directory or doesn't exist.
// Only works with local filesystem paths.
func (f *FNS) IsFile(ctx context.Context, path string) (bool, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return false, err
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	return f.stratFor(path).IsFile(ctx, path)
}

// Read reads the entire content of a resource into memory as a byte slice.
// Returns the complete file content or downloaded data or a ReadCloser for large files.
// Use ReadStream for large files to avoid memory issues.
func (f *FNS) Read(ctx context.Context, path string) ([]byte, io.ReadCloser, error) {
	// IDEA: what if we just use ReadStream internally and read all bytes from it?
	// So we just read the whole thing into memory ONLY if it's small enough <-- (how to define small enough?)
	// idk let's say 20MB for now
	// PLACEHOLDER is Maxsize constant

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	// get absolute path for local files
	abspath, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}

	// get info to check size
	info, err := f.GetInfo(ctx, abspath)
	if err != nil {
		return nil, nil, err
	}

	// check if it's a directory
	if info.Type == "dir" {
		return nil, nil, fmt.Errorf("path is a directory, cannot read: %s", path)
	}

	// Check for context cancellation before reading
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// use ReadStream to get a ReadCloser and decide based on size
	rc, err := f.stratFor(path).ReadStream(ctx, path)
	if err != nil {
		return nil, nil, err
	}

	if info.Size > Maxsize {
		return nil, rc, nil // Return ReadCloser for streaming (as if ReadStream had been called directly)
	} else {
		defer rc.Close()
		data, err := io.ReadAll(rc) // Read entire content into memory
		if err != nil {
			return nil, nil, err
		}
		return data, nil, nil
	}
}

// ReadStream returns an io.ReadCloser for streaming data from a resource.
// Preferred for large files as it doesn't load everything into memory.
// Caller must close the returned ReadCloser when done.
func (f *FNS) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return f.stratFor(path).ReadStream(ctx, path)
}

// Write writes data to a resource, creating the file if it doesn't exist.
// Overwrites existing files. Only works with local filesystem paths.
// Use WriteStream for large data to avoid memory issues.
func (f *FNS) Write(ctx context.Context, path string, data []byte) error {

	return f.stratFor(path).Write(ctx, path, data)
}

// WriteStream writes data from an io.Reader to a resource.
// Preferred for large data as it streams without loading everything into memory.
// Only works with local filesystem paths.
func (f *FNS) WriteStream(ctx context.Context, path string, reader io.Reader) error {

	return f.stratFor(path).WriteStream(ctx, path, reader)
}

// Append appends data to the end of a resource, creating the file if it doesn't exist.
// Only works with local filesystem paths.
func (f *FNS) Append(ctx context.Context, path string, data []byte) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return f.stratFor(path).Append(ctx, path, data)
}

// List returns a slice of ResourceInfo for all items in a directory.
// Only works with local filesystem paths.
func (f *FNS) List(ctx context.Context, path string) ([]ResourceInfo, error) {

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return f.stratFor(path).List(ctx, path)
}

// Mkdir creates a single directory with the specified permissions.
// Fails if parent directories don't exist. Only works with local filesystem paths.
func (f *FNS) Mkdir(ctx context.Context, path string, perm os.FileMode) error {

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return f.stratFor(path).Mkdir(ctx, path, perm)
}

// MkdirAll creates a directory and all necessary parent directories with the specified permissions.
// Creates parent directories as needed. Only works with local filesystem paths.
func (f *FNS) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {

	return f.stratFor(path).MkdirAll(ctx, path, perm)
}

// Remove deletes a single file or empty directory.
// Fails if the directory is not empty. Only works with local filesystem paths.
func (f *FNS) Remove(ctx context.Context, path string) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return f.stratFor(path).Remove(ctx, path)
}

// RemoveAll deletes a file or directory and all its contents recursively.
// Use with caution as it permanently deletes everything. Only works with local filesystem paths.
func (f *FNS) RemoveAll(ctx context.Context, path string) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return f.stratFor(path).RemoveAll(ctx, path)
}

// Copy copies a resource from source to destination.
// Works with local files and can download from URLs to local destinations.
func (f *FNS) Copy(ctx context.Context, src, dst string) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	// Prevent copying to remote URLs
	if strings.HasPrefix(dst, "http://") || strings.HasPrefix(dst, "https://") {
		return fmt.Errorf("Copy to remote URLs not supported")
	}

	return f.stratFor(src).Copy(ctx, src, dst)
}

// Move moves a resource from source to destination.
// Equivalent to copy + remove. Only works with local filesystem paths.
func (f *FNS) Move(ctx context.Context, src, dst string) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return f.stratFor(src).Move(ctx, src, dst)
}

// Rename renames a resource from source to destination.
// Alias for Move. Only works with local filesystem paths.
func (f *FNS) Rename(ctx context.Context, src, dst string) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return f.stratFor(src).Rename(ctx, src, dst)
}

// Chmod changes the file permissions of a resource.
// Only works with local filesystem paths.
func (f *FNS) Chmod(ctx context.Context, path string, mode os.FileMode) error {

	return f.stratFor(path).Chmod(ctx, path, mode)
}

// Chown changes the ownership of a resource (user ID and group ID).
// Only works with local filesystem paths and requires appropriate permissions.
func (f *FNS) Chown(ctx context.Context, path string, uid, gid int) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// On windows, Chown is a no-op
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Chown not supported on Windows") // or could just return nil to silently ignore
	}

	return f.stratFor(path).Chown(ctx, path, uid, gid)
}

// Download downloads a resource from a URL to a local destination path.
// The progress callback receives the number of bytes downloaded.
func (f *FNS) Download(ctx context.Context, url, dst string, progress func(int)) error {

	if f.stratFor(dst) != f.localStrat {
		return fmt.Errorf("Download destination must be a local path")
	}

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return f.stratFor(url).Download(ctx, url, dst, progress)
}

// DownloadStream returns an io.ReadCloser for streaming a download from a URL.
// The progress callback receives the number of bytes downloaded.
// Caller must close the returned ReadCloser when done.
func (f *FNS) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return f.stratFor(url).DownloadStream(ctx, url, progress)
}

// Fetch downloads content from a URL and returns it as a byte slice.
// Use for small resources. For large downloads, use DownloadStream.
func (f *FNS) Fetch(ctx context.Context, url string) ([]byte, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return f.stratFor(url).Fetch(ctx, url)
}

// CacheGet retrieves data from the cache using the specified key.
// Returns an error if the key doesn't exist or has expired.
func (f *FNS) CacheGet(ctx context.Context, key string) ([]byte, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	it, found := f.cache.Get(key)
	if !found {
		return nil, fmt.Errorf("cache miss for key: %s", key)
	}
	return it, nil
}

// CacheSet stores data in the cache with the specified key and time-to-live (TTL).
// Data will be automatically removed after the TTL expires.
func (f *FNS) CacheSet(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	f.cache.Set(key, data, ttl)
	return nil
}

// CacheDelete removes data from the cache using the specified key.
// No error is returned if the key doesn't exist.
func (f *FNS) CacheDelete(ctx context.Context, key string) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	f.cache.Delete(key)
	return nil
}

// CacheClear removes all data from the cache.
// Use with caution as this permanently deletes all cached data.
func (f *FNS) CacheClear(ctx context.Context) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	f.cache.Clear()
	return nil
}

// Resolve determines the actual path and resource type for a given path or URL.
// Returns the resolved path, resource type, and any error encountered.
func (f *FNS) Resolve(ctx context.Context, path string) (string, ResourceType, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	default:
	}

	info, err := f.GetInfo(ctx, path)
	if err != nil {
		return "", "", err
	}
	var absPath string

	if f.stratFor(path) == f.localStrat {
		absPath, err = filepath.Abs(path)
		if err != nil {
			return "", "", err
		}
		return absPath, info.Type, nil
	} else {
		return path, info.Type, nil
	}
}

// Validate checks if a path or URL is valid and accessible.
// Returns an error if the resource is invalid, inaccessible, or blocked by policy.
func (f *FNS) Validate(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return f.stratFor(path).Validate(ctx, path)
}

// TempFile creates a temporary file with the specified pattern and returns its path.
// The file is created in the system's temporary directory.
func (f *FNS) TempFile(ctx context.Context, pattern string) (string, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return "", err
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, pattern)
	if err != nil {
		return "", err
	}

	defer tmpFile.Close()

	return tmpFile.Name(), nil
}

// TempDir creates a temporary directory with the specified pattern and returns its path.
// The directory is created in the system's temporary directory.
func (f *FNS) TempDir(ctx context.Context, pattern string) (string, error) {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return "", err
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	dir := os.TempDir()

	tmpDir, err := os.MkdirTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	return tmpDir, nil
}

// Walk recursively traverses a directory tree, calling the provided function for each file and directory.
// The callback function receives the path, ResourceInfo, and any error encountered.
// Return an error from the callback to stop the walk, or nil to continue.
func (f *FNS) Walk(ctx context.Context, root string, fn func(path string, info ResourceInfo, err error) error) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Normalize root path
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		if cbErr := fn(root, ResourceInfo{}, fmt.Errorf("invalid root path: %w", err)); cbErr != nil {
			return cbErr
		}
		return err
	}

	// Get initial stat for root
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		if cbErr := fn(rootAbs, ResourceInfo{}, fmt.Errorf("failed to stat root: %w", err)); cbErr != nil {
			return cbErr
		}
		return err
	}

	// helper function to convert os.FileInfo to ResourceInfo
	convertInfo := func(p string) ResourceInfo {
		info, _ := f.localStrat.GetInfo(ctx, p)
		return *info
	}

	// call callback for root
	if cbErr := fn(rootAbs, convertInfo(rootAbs), nil); cbErr != nil {
		return cbErr
	}

	// Check if root is a directory
	if !rootInfo.IsDir() {
		return nil // nothing more to do
	}

	// Walk the directory tree
	walkFn := func(p string, d os.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// if os error, pass to callback
		if err != nil {
			return fn(p, ResourceInfo{}, fmt.Errorf("error accessing path: %w", err))
		}

		info := convertInfo(p)

		// call the callback
		if cbErr := fn(p, info, nil); cbErr != nil {
			return cbErr
		}

		if d.Type()&os.ModeSymlink != 0 {
			// Resolve symlink target
			targetInfo, err := os.Lstat(p)
			if err == nil && targetInfo.IsDir() {
				return fs.SkipDir // skip if cannot stat target
			}
		}
		return nil
	}
	return filepath.WalkDir(rootAbs, walkFn)
}
