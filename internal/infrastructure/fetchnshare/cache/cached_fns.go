package cache

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare"
)

type CachedFNS struct {
	wrapped fetchnshare.FNSInterface
	cache   *Cache
	ttl     time.Duration
}

func NewCachedFNS(wrapped fetchnshare.FNSInterface, ttl time.Duration) fetchnshare.FNSInterface {
	return &CachedFNS{
		wrapped: wrapped,
		cache:   New(),
		ttl:     ttl,
	}
}

func (c *CachedFNS) GetInfo(ctx context.Context, path string) (*fetchnshare.ResourceInfo, error) {
	return c.wrapped.GetInfo(ctx, path)
}

func (c *CachedFNS) Exists(ctx context.Context, path string) (bool, error) {
	return c.wrapped.Exists(ctx, path)
}

func (c *CachedFNS) IsDir(ctx context.Context, path string) (bool, error) {
	return c.wrapped.IsDir(ctx, path)
}

func (c *CachedFNS) IsFile(ctx context.Context, path string) (bool, error) {
	return c.wrapped.IsFile(ctx, path)
}

func (c *CachedFNS) Read(ctx context.Context, path string) ([]byte, io.ReadCloser, error) {
	cacheKey := fmt.Sprintf("read:%s", path)

	if data, found := c.cache.Get(cacheKey); found {
		return data, nil, nil
	}

	data, stream, err := c.wrapped.Read(ctx, path)
	if err != nil {
		return nil, nil, err
	}

	if data != nil {
		c.cache.Set(cacheKey, data, c.ttl)
	}

	return data, stream, nil
}

func (c *CachedFNS) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	return c.wrapped.ReadStream(ctx, path)
}

func (c *CachedFNS) Write(ctx context.Context, path string, data []byte) error {
	c.cache.Delete(fmt.Sprintf("read:%s", path))
	return c.wrapped.Write(ctx, path, data)
}

func (c *CachedFNS) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	c.cache.Delete(fmt.Sprintf("read:%s", path))
	return c.wrapped.WriteStream(ctx, path, reader)
}

func (c *CachedFNS) Append(ctx context.Context, path string, data []byte) error {
	c.cache.Delete(fmt.Sprintf("read:%s", path))
	return c.wrapped.Append(ctx, path, data)
}

func (c *CachedFNS) List(ctx context.Context, path string) ([]fetchnshare.ResourceInfo, error) {
	return c.wrapped.List(ctx, path)
}

func (c *CachedFNS) Mkdir(ctx context.Context, path string, perm os.FileMode) error {
	return c.wrapped.Mkdir(ctx, path, perm)
}

func (c *CachedFNS) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	return c.wrapped.MkdirAll(ctx, path, perm)
}

func (c *CachedFNS) Remove(ctx context.Context, path string) error {
	c.cache.Delete(fmt.Sprintf("read:%s", path))
	return c.wrapped.Remove(ctx, path)
}

func (c *CachedFNS) RemoveAll(ctx context.Context, path string) error {
	c.cache.Clear()
	return c.wrapped.RemoveAll(ctx, path)
}

func (c *CachedFNS) Copy(ctx context.Context, src, dst string) error {
	return c.wrapped.Copy(ctx, src, dst)
}

func (c *CachedFNS) Move(ctx context.Context, src, dst string) error {
	c.cache.Delete(fmt.Sprintf("read:%s", src))
	return c.wrapped.Move(ctx, src, dst)
}

func (c *CachedFNS) Rename(ctx context.Context, src, dst string) error {
	c.cache.Delete(fmt.Sprintf("read:%s", src))
	return c.wrapped.Rename(ctx, src, dst)
}

func (c *CachedFNS) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	return c.wrapped.Chmod(ctx, path, mode)
}

func (c *CachedFNS) Chown(ctx context.Context, path string, uid, gid int) error {
	return c.wrapped.Chown(ctx, path, uid, gid)
}

func (c *CachedFNS) Download(ctx context.Context, url, dst string, progress func(int)) error {
	return c.wrapped.Download(ctx, url, dst, progress)
}

func (c *CachedFNS) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	return c.wrapped.DownloadStream(ctx, url, progress)
}

func (c *CachedFNS) Fetch(ctx context.Context, url string) ([]byte, error) {
	cacheKey := fmt.Sprintf("fetch:%s", url)

	if data, found := c.cache.Get(cacheKey); found {
		return data, nil
	}

	data, err := c.wrapped.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}

	c.cache.Set(cacheKey, data, c.ttl)
	return data, nil
}

func (c *CachedFNS) Resolve(ctx context.Context, path string) (string, fetchnshare.ResourceType, error) {
	return c.wrapped.Resolve(ctx, path)
}

func (c *CachedFNS) Validate(ctx context.Context, path string) error {
	return c.wrapped.Validate(ctx, path)
}

func (c *CachedFNS) TempFile(ctx context.Context, pattern string) (string, error) {
	return c.wrapped.TempFile(ctx, pattern)
}

func (c *CachedFNS) TempDir(ctx context.Context, pattern string) (string, error) {
	return c.wrapped.TempDir(ctx, pattern)
}

func (c *CachedFNS) Walk(ctx context.Context, root string, fn func(path string, info fetchnshare.ResourceInfo, err error) error) error {
	return c.wrapped.Walk(ctx, root, fn)
}
