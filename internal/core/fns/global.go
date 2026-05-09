package fns

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/fns/strategies"
)

func getStrategy(path string, opts ...config.Option) FNSInterface {
	cfg := config.Default()
	for _, opt := range opts {
		opt(&cfg)
	}

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return strategies.NewRemote(cfg)
	}

	return strategies.NewLocal(cfg)
}

func GetInfo(
	ctx context.Context,
	path string,
	opts ...config.Option,
) (int64, string, time.Time, error) {
	return getStrategy(path, opts...).GetInfo(ctx, path)
}

func Exists(
	ctx context.Context,
	path string,
	opts ...config.Option,
) (bool, error) {
	return getStrategy(path, opts...).Exists(ctx, path)
}

func IsDir(
	ctx context.Context,
	path string,
	opts ...config.Option,
) (bool, error) {
	return getStrategy(path, opts...).IsDir(ctx, path)
}

func IsFile(
	ctx context.Context,
	path string,
	opts ...config.Option,
) (bool, error) {
	return getStrategy(path, opts...).IsFile(ctx, path)
}

func Read(
	ctx context.Context,
	path string,
	opts ...config.Option,
) ([]byte, error) {
	return getStrategy(path, opts...).Read(ctx, path)
}

func ReadStream(
	ctx context.Context,
	path string,
	opts ...config.Option,
) (io.ReadCloser, error) {
	return getStrategy(path, opts...).ReadStream(ctx, path)
}

func Write(
	ctx context.Context,
	path string,
	data []byte,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).Write(ctx, path, data)
}

func WriteStream(
	ctx context.Context,
	path string,
	reader io.Reader,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).WriteStream(ctx, path, reader)
}

func Append(
	ctx context.Context,
	path string,
	data []byte,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).Append(ctx, path, data)
}

func List(
	ctx context.Context,
	path string,
	opts ...config.Option,
) ([]string, error) {
	return getStrategy(path, opts...).List(ctx, path)
}

func Mkdir(
	ctx context.Context,
	path string,
	perm os.FileMode,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).Mkdir(ctx, path, perm)
}

func MkdirAll(
	ctx context.Context,
	path string,
	perm os.FileMode,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).MkdirAll(ctx, path, perm)
}

func Remove(
	ctx context.Context,
	path string,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).Remove(ctx, path)
}

func RemoveAll(
	ctx context.Context,
	path string,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).RemoveAll(ctx, path)
}

func Copy(
	ctx context.Context,
	src, dst string,
	opts ...config.Option,
) error {
	return getStrategy(src, opts...).Copy(ctx, src, dst)
}

func Move(
	ctx context.Context,
	src, dst string,
	opts ...config.Option,
) error {
	return getStrategy(src, opts...).Move(ctx, src, dst)
}

func Rename(
	ctx context.Context,
	src, dst string,
	opts ...config.Option,
) error {
	return getStrategy(src, opts...).Rename(ctx, src, dst)
}

func Chmod(
	ctx context.Context,
	path string,
	mode os.FileMode,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).Chmod(ctx, path, mode)
}

func Chown(
	ctx context.Context,
	path string,
	uid, gid int,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).Chown(ctx, path, uid, gid)
}

func Download(
	ctx context.Context,
	url, dst string,
	progress func(int),
	opts ...config.Option,
) error {
	return getStrategy(url, opts...).Download(ctx, url, dst, progress)
}

func DownloadStream(
	ctx context.Context,
	url string,
	progress func(int),
	opts ...config.Option,
) (io.ReadCloser, error) {
	return getStrategy(url, opts...).DownloadStream(ctx, url, progress)
}

func Fetch(
	ctx context.Context,
	url string,
	opts ...config.Option,
) ([]byte, error) {
	return getStrategy(url, opts...).Fetch(ctx, url)
}

func Validate(
	ctx context.Context,
	path string,
	opts ...config.Option,
) error {
	return getStrategy(path, opts...).Validate(ctx, path)
}
