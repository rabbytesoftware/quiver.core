package fetchnshare

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// RemoteStrategy handles remote URL operations
type RemoteStrategy struct{}

func (r *RemoteStrategy) getResp(ctx context.Context, cmd string, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, cmd, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check URL existence: %w", err)
	}
	return resp, nil
}

func (r *RemoteStrategy) GetInfo(ctx context.Context, path string) (*ResourceInfo, error) {

	resp, err := r.getResp(ctx, "GET", path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	info := &ResourceInfo{}
	info.Type = ResourceType(http.DetectContentType([]byte(path)))

	info.Size = resp.ContentLength // May be -1 if unknown
	if info.Size < 0 {

		var total int64 // var used in either Content-Range or full body read

		// If Content-Length doesn't exist, try Content-Range
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			var req *http.Request
			req, _ = http.NewRequestWithContext(ctx, "GET", path, nil)
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

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

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
	return info, nil
}

func (r *RemoteStrategy) Exists(ctx context.Context, path string) (bool, error) {

	resp, err := r.getResp(ctx, "HEAD", path)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode <= 400, nil
}

func (r *RemoteStrategy) IsDir(ctx context.Context, path string) (bool, error) { // Not applicable for remote URLs
	return false, fmt.Errorf("IsDir not supported for remote URLs")
}

func (r *RemoteStrategy) IsFile(ctx context.Context, path string) (bool, error) { // Not applicable for remote URLs
	return false, fmt.Errorf("IsFile not supported for remote URLs")
}

func (r *RemoteStrategy) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {

	resp, err := r.getResp(ctx, "GET", path)
	if err != nil {
		return nil, err
	}

	// Check for non-200 status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("remote resource returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func (r *RemoteStrategy) Write(ctx context.Context, path string, data []byte) error { // Not supported for remote URLs
	return fmt.Errorf("Write not supported for remote URLs")
}

func (r *RemoteStrategy) WriteStream(ctx context.Context, path string, reader io.Reader) error { // Not supported for remote URLs
	return fmt.Errorf("WriteStream not supported for remote URLs")
}

func (r *RemoteStrategy) Append(ctx context.Context, path string, data []byte) error { // Not supported for remote URLs
	return fmt.Errorf("Append not supported for remote URLs")
}

func (r *RemoteStrategy) List(ctx context.Context, path string) ([]ResourceInfo, error) { // Not supported for remote URLs
	return nil, fmt.Errorf("List not supported for remote URLs")
}

func (r *RemoteStrategy) Mkdir(ctx context.Context, path string, perm os.FileMode) error { // Not supported for remote URLs
	return fmt.Errorf("Mkdir not supported for remote URLs")
}

func (r *RemoteStrategy) MkdirAll(ctx context.Context, path string, perm os.FileMode) error { // Not supported for remote URLs
	return fmt.Errorf("MkdirAll not supported for remote URLs")
}

func (r *RemoteStrategy) Remove(ctx context.Context, path string) error { // Not supported for remote URLs
	return fmt.Errorf("Remove not supported for remote URLs")
}

func (r *RemoteStrategy) RemoveAll(ctx context.Context, path string) error { // Not supported for remote URLs
	return fmt.Errorf("RemoveAll not supported for remote URLs")
}

func (r *RemoteStrategy) Copy(ctx context.Context, src, dst string) error {
	var srcReader io.ReadCloser
	var err error

	resp, err := r.getResp(ctx, "GET", src)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	srcReader = resp.Body

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

func (r *RemoteStrategy) Move(ctx context.Context, src, dst string) error { // Not supported for remote URLs
	return fmt.Errorf("Move not supported for remote URLs")
}

func (r *RemoteStrategy) Rename(ctx context.Context, src, dst string) error { // Not supported for remote URLs
	return fmt.Errorf("Rename not supported for remote URLs")
}

func (r *RemoteStrategy) Chmod(ctx context.Context, path string, mode os.FileMode) error { // Not supported for remote URLs
	return fmt.Errorf("Chmod not supported for remote URLs")
}

func (r *RemoteStrategy) Chown(ctx context.Context, path string, uid, gid int) error { // Not supported for remote URLs
	return fmt.Errorf("Chown not supported for remote URLs")
}

func (r *RemoteStrategy) Download(ctx context.Context, url, dst string, progress func(int)) error {

	var source io.ReadCloser

	info, err := r.GetInfo(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to get URL info: %w", err)
	}

	if info.Size < Maxsize {
		resp, err := r.getResp(ctx, "GET", url)
		if err != nil {
			return err
		}

		source = resp.Body
	} else {
		reader, err := r.DownloadStream(ctx, url, progress)
		if err != nil {
			return fmt.Errorf("failed to download stream: %w", err)
		}

		source = reader
	}
	defer source.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	defer dstFile.Close()

	buf := make([]byte, 32*1024)
	var totalWritten int
	for {
		n, err := source.Read(buf)
		if n > 0 {
			written, wErr := dstFile.Write(buf[:n])
			if wErr != nil {
				return fmt.Errorf("failed to write to destination: %w", wErr)
			}
			totalWritten += written

			// Report progress if callback is provided
			if progress != nil {
				progress(totalWritten)
			}

			// Check for context cancellation
			if err := ctx.Err(); err != nil {
				return err
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading from source: %w", err)
		}
	}
	return nil
}

func (r *RemoteStrategy) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {

	resp, err := r.getResp(ctx, "GET", url)
	if err != nil {
		return nil, err
	}

	// Check for non-200 status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("remote resource returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func (r *RemoteStrategy) Fetch(ctx context.Context, url string) ([]byte, error) {

	resp, err := r.getResp(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for non-200 status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote resource returned status %d", resp.StatusCode)
	}

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Read entire body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return data, nil
}

func (r *RemoteStrategy) Validate(ctx context.Context, path string) error {

	exists, err := r.Exists(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to validate URL: %w", err)
	}
	if !exists {
		return fmt.Errorf("URL does not exist: %s", path)
	}

	resp, err := r.getResp(ctx, "HEAD", path)
	if err != nil {
		return fmt.Errorf("failed to validate URL accessibility: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("URL is not accessible, status code: %d", resp.StatusCode)
	}

	return nil
}
