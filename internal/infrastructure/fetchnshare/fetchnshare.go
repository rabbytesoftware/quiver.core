package fetchnshare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxSize = 20 * 1024 * 1024 // 20MB

type FNS struct {
}

func NewFNS() FNSInterface {
	return &FNS{}
}

// GetInfo retrieves metadata information about a resource (file or directory).
// It returns ResourceInfo containing size, permissions, modification time, and other attributes.
// Supports both local filesystem paths and remote URLs (HTTP/HTTPS).
func (f *FNS) GetInfo(ctx context.Context, path string) (*ResourceInfo, error) {

	info := &ResourceInfo{Path: path}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Remote URLs
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {

		req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)

		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch URL info: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		defer resp.Body.Close()

		info.Type = ResourceType(http.DetectContentType([]byte(path)))

		info.Size = resp.ContentLength // May be -1 if unknown
		if info.Size < 0 {

			var total int64 // var used in either Content-Range or full body read

			// If Content-Length doesn't exist, try Content-Range
			if cr := resp.Header.Get("Content-Range"); cr != "" {
				req.Header.Set("Range", "bytes=0-0")
				conRange := resp.Header.Get("Content-Range")

				_, err := fmt.Sscanf(conRange, "bytes 0-0/%d", &total)
				if err == nil && total > 0 {
					info.Size = total
				}
			} else {
				// As a last resort, read the entire body to determine size

				buf := make([]byte, 32*1024)
				for {
					n, err := resp.Body.Read(buf)
					total += int64(n)
					if err == io.EOF {
						break
					}
				}
				info.Size = total
			}
		}

		tim := resp.Header.Get("Last-Modified")
		modT, err := http.ParseTime(tim)
		if tim == "" || err != nil { // If parsing fails or header is missing
			info.ModTime = time.Time{} // Unknown mod time ( or could use time.Now()? )
		} else {
			info.ModTime = modT
		}

	} else { // Local filesystem paths

		stat, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		info.Size = stat.Size()
		info.ModTime = stat.ModTime()
		if stat.IsDir() { // Maybe check for more types?
			info.Type = ResourceType("directory")
		} else {
			info.Type = ResourceType("file")
		}
	}

	return info, nil
}

// Exists checks whether a resource exists at the given path.
// Returns true if the resource exists, false otherwise.
// Works with both local filesystem paths and remote URLs.
func (f *FNS) Exists(ctx context.Context, path string) (bool, error) {

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Remote URLs
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {

		req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
		if err != nil {
			return false, fmt.Errorf("failed to create request: %w", err)

		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false, fmt.Errorf("failed to fetch URL info: %w", err)
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 { // 2xx and 3xx means it exists
			return true, nil
		}

		if resp.StatusCode == http.StatusNotFound { // 404 means it doesn't exist
			return false, nil
		}

		return false, fmt.Errorf("unexpected HTTP status: %s", resp.Status)

	} else { // Local filesystem paths

		_, err := os.Stat(path)
		if err == nil {
			return true, nil
		}

		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}
}

// IsDir checks whether the resource at the given path is a directory.
// Returns true if the resource is a directory, false if it's a file or doesn't exist.
// Only works with local filesystem paths.
func (f *FNS) IsDir(ctx context.Context, path string) (bool, error) {

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return false, fmt.Errorf("IsDir not supported for remote URLs")
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, nil
	}

	if info.IsDir() {
		return true, nil
	} else {
		return false, nil
	}
}

// IsFile checks whether the resource at the given path is a regular file.
// Returns true if the resource is a file, false if it's a directory or doesn't exist.
// Only works with local filesystem paths.
func (f *FNS) IsFile(ctx context.Context, path string) (bool, error) {

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return false, fmt.Errorf("IsFile not supported for remote URLs")
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, nil
	}

	if info.IsDir() {
		return false, nil
	} else {
		return true, nil
	}
}

// Read reads the entire content of a resource into memory as a byte slice.
// Returns the complete file content or downloaded data.
// Use ReadStream for large files to avoid memory issues.
func (f *FNS) Read(ctx context.Context, path string) ([]byte, io.ReadCloser, error) {
	// IDEA: what if we just use ReadStream internally and read all bytes from it?
	// Also we don't have to duplicate the URL vs local file logic.

	// So we just read the whole thing into memory ONLY if it's small enough <-- (how to define small enough?)
	// idk let's say 20MB for now

	info, err := f.GetInfo(ctx, path)

	if err != nil {
		return nil, nil, err
	}

	if check, err := f.IsDir(ctx, path); err != nil {
		return nil, nil, err
	} else if check {
		return nil, nil, fmt.Errorf("path is a directory, not a file")
	}

	rc, err := f.ReadStream(ctx, path)
	if err != nil {
		return nil, nil, err
	}

	// Check for context cancellation
	select {
	case <-ctx.Done():
		rc.Close()
		return nil, nil, ctx.Err()
	default:
	}

	if info.Size > maxSize {
		return nil, rc, nil // Return ReadCloser for streaming (as if ReadStream was called directly)
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

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		// For remote URLs
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch remote resource: %w", err)
		}

		// Check for non-200 status codes
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			return nil, fmt.Errorf("remote resource returned status %d", resp.StatusCode)
		}

		return resp.Body, nil
	}

	// For local files
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open local file: %w", err)
	}

	return file, nil
}

// Write writes data to a resource, creating the file if it doesn't exist.
// Overwrites existing files. Only works with local filesystem paths.
// Use WriteStream for large data to avoid memory issues.
func (f *FNS) Write(ctx context.Context, path string, data []byte) error {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return fmt.Errorf("IsFile not supported for remote URLs")
	}

	// Ensure parent directories exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// If data is too large, use WriteStream
	if len(data) > maxSize {
		return f.WriteStream(ctx, path, bytes.NewReader(data))
	} else {

		// Write data to file
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
	}

	return nil
}

// WriteStream writes data from an io.Reader to a resource.
// Preferred for large data as it streams without loading everything into memory.
// Only works with local filesystem paths.
func (f *FNS) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return fmt.Errorf("IsFile not supported for remote URLs")
	}
	// Check directory and file existence here as well since this can be run standalone or via Write()

	// Ensure parent directories exist
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

// Append appends data to the end of a resource, creating the file if it doesn't exist.
// Only works with local filesystem paths.
func (f *FNS) Append(ctx context.Context, path string, data []byte) error {
	// Remote URLs not supported
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return fmt.Errorf("Append not supported for remote URLs")
	}

	// Ensure parent directories exist
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

// List returns a slice of ResourceInfo for all items in a directory.
// Only works with local filesystem paths.
func (f *FNS) List(ctx context.Context, path string) ([]ResourceInfo, error) {

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Remote URLs not supported
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return nil, fmt.Errorf("List not supported for remote URLs")
	}

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
		info, err := f.GetInfo(ctx, entryPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get info for %s: %w", entryPath, err)
		}

		resources = append(resources, *info)
	}

	return resources, nil
}

// Mkdir creates a single directory with the specified permissions.
// Fails if parent directories don't exist. Only works with local filesystem paths.
func (f *FNS) Mkdir(ctx context.Context, path string, perm os.FileMode) error {

	// Remote URLs not supported
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return fmt.Errorf("Mkdir not supported for remote URLs")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Create the directory
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	} else {
		return nil
	}
}

// MkdirAll creates a directory and all necessary parent directories with the specified permissions.
// Creates parent directories as needed. Only works with local filesystem paths.
func (f *FNS) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	return nil
}

// Remove deletes a single file or empty directory.
// Fails if the directory is not empty. Only works with local filesystem paths.
func (f *FNS) Remove(ctx context.Context, path string) error {
	return nil
}

// RemoveAll deletes a file or directory and all its contents recursively.
// Use with caution as it permanently deletes everything. Only works with local filesystem paths.
func (f *FNS) RemoveAll(ctx context.Context, path string) error {
	return nil
}

// Copy copies a resource from source to destination.
// Works with local files and can download from URLs to local destinations.
func (f *FNS) Copy(ctx context.Context, src, dst string) error {
	return nil
}

// Move moves a resource from source to destination.
// Equivalent to copy + remove. Only works with local filesystem paths.
func (f *FNS) Move(ctx context.Context, src, dst string) error {
	return nil
}

// Rename renames a resource from source to destination.
// Alias for Move. Only works with local filesystem paths.
func (f *FNS) Rename(ctx context.Context, src, dst string) error {
	return nil
}

// Chmod changes the file permissions of a resource.
// Only works with local filesystem paths.
func (f *FNS) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	return nil
}

// Chown changes the ownership of a resource (user ID and group ID).
// Only works with local filesystem paths and requires appropriate permissions.
func (f *FNS) Chown(ctx context.Context, path string, uid, gid int) error {
	return nil
}

// Download downloads a resource from a URL to a local destination path.
// The progress callback receives the number of bytes downloaded.
func (f *FNS) Download(ctx context.Context, url, dst string, progress func(int)) error {
	return nil
}

// DownloadStream returns an io.ReadCloser for streaming a download from a URL.
// The progress callback receives the number of bytes downloaded.
// Caller must close the returned ReadCloser when done.
func (f *FNS) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	return nil, nil
}

// Fetch downloads content from a URL and returns it as a byte slice.
// Use for small resources. For large downloads, use DownloadStream.
func (f *FNS) Fetch(ctx context.Context, url string) ([]byte, error) {
	return nil, nil
}

// CacheGet retrieves data from the cache using the specified key.
// Returns an error if the key doesn't exist or has expired.
func (f *FNS) CacheGet(ctx context.Context, key string) ([]byte, error) {
	return nil, nil
}

// CacheSet stores data in the cache with the specified key and time-to-live (TTL).
// Data will be automatically removed after the TTL expires.
func (f *FNS) CacheSet(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	return nil
}

// CacheDelete removes data from the cache using the specified key.
// No error is returned if the key doesn't exist.
func (f *FNS) CacheDelete(ctx context.Context, key string) error {
	return nil
}

// CacheClear removes all data from the cache.
// Use with caution as this permanently deletes all cached data.
func (f *FNS) CacheClear(ctx context.Context) error {
	return nil
}

// Resolve determines the actual path and resource type for a given path or URL.
// Returns the resolved path, resource type, and any error encountered.
func (f *FNS) Resolve(ctx context.Context, path string) (string, ResourceType, error) {
	return "", "", nil
}

// Validate checks if a path or URL is valid and accessible.
// Returns an error if the resource is invalid, inaccessible, or blocked by policy.
func (f *FNS) Validate(ctx context.Context, path string) error {
	return nil
}

// TempFile creates a temporary file with the specified pattern and returns its path.
// The file is created in the system's temporary directory.
func (f *FNS) TempFile(ctx context.Context, pattern string) (string, error) {
	return "", nil
}

// TempDir creates a temporary directory with the specified pattern and returns its path.
// The directory is created in the system's temporary directory.
func (f *FNS) TempDir(ctx context.Context, pattern string) (string, error) {
	return "", nil
}

// Walk recursively traverses a directory tree, calling the provided function for each file and directory.
// The callback function receives the path, ResourceInfo, and any error encountered.
// Return an error from the callback to stop the walk, or nil to continue.
func (f *FNS) Walk(ctx context.Context, root string, fn func(path string, info ResourceInfo, err error) error) error {
	return nil
}
