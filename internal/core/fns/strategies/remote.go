package strategies

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/fns/config"
	"github.com/rabbytesoftware/quiver/internal/core/fns/errors"
)

type Remote struct {
	client     *http.Client
	bufferSize int
}

func NewRemote(cfg config.Config) *Remote {
	return &Remote{
		client:     cfg.HTTPClient,
		bufferSize: cfg.BufferSize,
	}
}

func (r *Remote) doRequest(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *Remote) GetInfo(ctx context.Context, path string) (int64, string, time.Time, error) {
	resp, err := r.doRequest(ctx, "GET", path)
	if err != nil {
		return 0, "", time.Time{}, errors.Op("GetInfo", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, "", time.Time{}, errors.Op("GetInfo", path, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	size := resp.ContentLength
	if size < 0 { //nolint:nestif
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			var total int64
			req, _ := http.NewRequestWithContext(ctx, "GET", path, nil)
			req.Header.Set("Range", "bytes=0-0")
			conRange := resp.Header.Get("Content-Range")
			_, err := fmt.Sscanf(conRange, "bytes 0-0/%d", &total)
			if err == nil && total > 0 {
				size = total
			}
		} else {
			buf := make([]byte, r.bufferSize)
			var total int64
			for {
				n, err := resp.Body.Read(buf)
				total += int64(n)

				select {
				case <-ctx.Done():
					return 0, "", time.Time{}, ctx.Err()
				default:
				}

				if err == io.EOF {
					break
				}
				if err != nil {
					return 0, "", time.Time{}, errors.Op("GetInfo", path, err)
				}
			}
			size = total
		}
	}

	modTime := time.Time{}
	if tim := resp.Header.Get("Last-Modified"); tim != "" {
		if parsed, err := http.ParseTime(tim); err == nil {
			modTime = parsed
		}
	}

	return size, "file", modTime, nil
}

func (r *Remote) Exists(ctx context.Context, path string) (bool, error) {
	resp, err := r.doRequest(ctx, "HEAD", path)
	if err != nil {
		return false, errors.Op("Exists", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	return resp.StatusCode >= 200 && resp.StatusCode < 400, nil
}

func (r *Remote) IsDir(ctx context.Context, path string) (bool, error) {
	return false, errors.Unsupported("IsDir", "remote URLs")
}

func (r *Remote) IsFile(ctx context.Context, path string) (bool, error) {
	return false, errors.Unsupported("IsFile", "remote URLs")
}

func (r *Remote) Read(ctx context.Context, path string) ([]byte, error) {
	return r.Fetch(ctx, path)
}

func (r *Remote) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	resp, err := r.doRequest(ctx, "GET", path)
	if err != nil {
		return nil, errors.Op("ReadStream", path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, errors.Op("ReadStream", path, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	return resp.Body, nil
}

func (r *Remote) Write(ctx context.Context, path string, data []byte) error {
	return errors.Unsupported("Write", "remote URLs")
}

func (r *Remote) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	return errors.Unsupported("WriteStream", "remote URLs")
}

func (r *Remote) Append(ctx context.Context, path string, data []byte) error {
	return errors.Unsupported("Append", "remote URLs")
}

func (r *Remote) List(ctx context.Context, path string) ([]string, error) {
	return nil, errors.Unsupported("List", "remote URLs")
}

func (r *Remote) Mkdir(ctx context.Context, path string, perm os.FileMode) error {
	return errors.Unsupported("Mkdir", "remote URLs")
}

func (r *Remote) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	return errors.Unsupported("MkdirAll", "remote URLs")
}

func (r *Remote) Remove(ctx context.Context, path string) error {
	return errors.Unsupported("Remove", "remote URLs")
}

func (r *Remote) RemoveAll(ctx context.Context, path string) error {
	return errors.Unsupported("RemoveAll", "remote URLs")
}

func (r *Remote) Copy(ctx context.Context, src, dst string) error {
	resp, err := r.doRequest(ctx, "GET", src)
	if err != nil {
		return errors.Op("Copy", src, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if dst == "" && strings.HasSuffix(dst, string(os.PathSeparator)) || strings.TrimSpace(dst) == "" || strings.ContainsRune(dst, 0) {
		return errors.Op("Copy", dst, fmt.Errorf("invalid destination"))
	}

	if resp.StatusCode != http.StatusOK {
		return errors.Op("Copy", src, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	cleanDst := filepath.Clean(dst)
	dstFile, err := os.Create(cleanDst)
	if err != nil {
		return errors.Op("Copy", cleanDst, err)
	}
	defer dstFile.Close() //nolint:errcheck

	if _, err = io.Copy(dstFile, resp.Body); err != nil {
		return errors.Op("Copy", cleanDst, err)
	}

	return nil
}

func (r *Remote) Move(ctx context.Context, src, dst string) error {
	return errors.Unsupported("Move", "remote URLs")
}

func (r *Remote) Rename(ctx context.Context, src, dst string) error {
	return errors.Unsupported("Rename", "remote URLs")
}

func (r *Remote) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	return errors.Unsupported("Chmod", "remote URLs")
}

func (r *Remote) Chown(ctx context.Context, path string, uid, gid int) error {
	return errors.Unsupported("Chown", "remote URLs")
}

func (r *Remote) Download(ctx context.Context, url, dst string, progress func(int)) error { //nolint:gocyclo
	size, _, _, err := r.GetInfo(ctx, url)
	if err != nil {
		return errors.Op("Download", url, err)
	}

	// validate destination
	if dst == "" && strings.HasSuffix(dst, string(os.PathSeparator)) || strings.TrimSpace(dst) == "" || strings.ContainsRune(dst, 0) {
		return errors.Op("Download", dst, fmt.Errorf("invalid destination"))
	}

	var source io.ReadCloser
	if size < config.DefaultMaxMemorySize { //nolint:nestif
		resp, err := r.doRequest(ctx, "GET", url) //nolint:bodyclose
		if err != nil {
			return errors.Op("Download", url, err)
		}
		source = resp.Body
	} else {
		reader, err := r.DownloadStream(ctx, url, progress)
		if err != nil {
			return err
		}
		source = reader
	}
	defer source.Close() //nolint:errcheck

	cleanDst := filepath.Clean(dst)
	dstFile, err := os.Create(cleanDst)
	if err != nil {
		return errors.Op("Download", cleanDst, err)
	}
	defer dstFile.Close() //nolint:errcheck

	buf := make([]byte, r.bufferSize)
	var totalWritten int
	for {
		n, err := source.Read(buf)
		if n > 0 { //nolint:nestif
			written, wErr := dstFile.Write(buf[:n])
			if wErr != nil {
				return errors.Op("Download", cleanDst, wErr)
			}
			totalWritten += written

			if progress != nil {
				progress(totalWritten)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Op("Download", url, err)
		}
	}

	return nil
}

func (r *Remote) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	resp, err := r.doRequest(ctx, "GET", url)
	if err != nil {
		return nil, errors.Op("DownloadStream", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, errors.Op("DownloadStream", url, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	return resp.Body, nil
}

func (r *Remote) Fetch(ctx context.Context, url string) ([]byte, error) {
	resp, err := r.doRequest(ctx, "GET", url)
	if err != nil {
		return nil, errors.Op("Fetch", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.Op("Fetch", url, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Op("Fetch", url, err)
	}

	return data, nil
}

func (r *Remote) Validate(ctx context.Context, path string) error {
	exists, err := r.Exists(ctx, path)
	if err != nil {
		return errors.Op("Validate", path, err)
	}
	if !exists {
		return errors.NotFound("Validate", path)
	}

	resp, err := r.doRequest(ctx, "HEAD", path)
	if err != nil {
		return errors.Op("Validate", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return errors.Op("Validate", path, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	return nil
}
