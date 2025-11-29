package fetchnshare

import (
	"context"
	"io"
	"os"
	"time"
)

type ResourceInfo struct {
	Path    string
	Type    ResourceType
	Size    int64
	ModTime time.Time
}

type ResourceType string

const (
	ResourceTypeFile ResourceType = "file"
	ResourceTypeDir  ResourceType = "dir"
)

type FNSInterface interface {
	// Resource Access
	GetInfo(
		ctx context.Context,
		path string,
	) (*ResourceInfo, error)
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

	// File Operations
	Read(
		ctx context.Context,
		path string,
	) ([]byte, io.ReadCloser, error)
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

	// Directory Operations
	List(
		ctx context.Context,
		path string,
	) ([]ResourceInfo, error)
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

	// File System Operations
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

	// Download and Fetch Operations
	Download(
		ctx context.Context,
		url,
		dst string,
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

	// Resource Resolution
	Resolve(
		ctx context.Context,
		path string,
	) (string, ResourceType, error)
	Validate(
		ctx context.Context,
		path string,
	) error

	// Utility Operations
	TempFile(
		ctx context.Context,
		pattern string,
	) (string, error)
	TempDir(
		ctx context.Context,
		pattern string,
	) (string, error)
	Walk(
		ctx context.Context,
		root string,
		fn func(path string, info ResourceInfo, err error) error,
	) error
}
