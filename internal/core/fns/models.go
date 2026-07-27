package fns

import (
	"context"
	"io"
	"os"
	"time"
)

type FNSInterface interface {
	GetInfo(
		ctx context.Context,
		path string,
	) (size int64, resourceType string, modTime time.Time, err error)
	Exists(
		ctx context.Context,
		path string,
	) (bool, error)
	IsDir(
		ctx context.Context,
		path string,
	) (bool, error)
	IsFile(
		ctx context.Context,
		path string,
	) (bool, error)
	ReadStream(
		ctx context.Context,
		path string,
	) (io.ReadCloser, error)
	Write(
		ctx context.Context,
		path string,
		data []byte,
	) error
	WriteStream(
		ctx context.Context,
		path string,
		reader io.Reader,
	) error
	Append(
		ctx context.Context,
		path string,
		data []byte,
	) error
	List(
		ctx context.Context,
		path string,
	) ([]string, error)
	Read(
		ctx context.Context,
		path string,
	) ([]byte, error)
	Mkdir(
		ctx context.Context,
		path string,
		perm os.FileMode,
	) error
	MkdirAll(
		ctx context.Context,
		path string,
		perm os.FileMode,
	) error
	Remove(
		ctx context.Context,
		path string,
	) error
	RemoveAll(
		ctx context.Context,
		path string,
	) error
	Copy(
		ctx context.Context,
		src, dst string,
	) error
	Move(
		ctx context.Context,
		src, dst string,
	) error
	Rename(
		ctx context.Context,
		src, dst string,
	) error
	Chmod(
		ctx context.Context,
		path string,
		mode os.FileMode,
	) error
	Chown(
		ctx context.Context,
		path string,
		uid, gid int,
	) error
	Download(
		ctx context.Context,
		url, dst string,
		progress func(int),
	) error
	DownloadStream(
		ctx context.Context,
		url string,
		progress func(int),
	) (io.ReadCloser, error)
	Fetch(
		ctx context.Context,
		url string,
	) ([]byte, error)
	Do(
		ctx context.Context,
		req Request,
	) (Response, error)
	Validate(
		ctx context.Context,
		path string,
	) error
}
