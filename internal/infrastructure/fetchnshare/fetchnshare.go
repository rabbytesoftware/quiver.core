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

	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/config"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/errors"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare/strategies"
)

type ResourceStrategy interface {
	GetInfo(ctx context.Context, path string) (size int64, resourceType string, modTime time.Time, err error)
	Exists(ctx context.Context, path string) (bool, error)
	IsDir(ctx context.Context, path string) (bool, error)
	IsFile(ctx context.Context, path string) (bool, error)
	ReadStream(ctx context.Context, path string) (io.ReadCloser, error)
	Write(ctx context.Context, path string, data []byte) error
	WriteStream(ctx context.Context, path string, reader io.Reader) error
	Append(ctx context.Context, path string, data []byte) error
	List(ctx context.Context, path string) ([]string, error)
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
	config      config.Config
}

func NewFNS(opts ...config.Option) FNSInterface {
	cfg := config.Default()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &FNS{
		localStrat:  strategies.NewLocal(cfg),
		remoteStrat: strategies.NewRemote(cfg),
		config:      cfg,
	}
}

func (f *FNS) stratFor(path string) ResourceStrategy {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return f.remoteStrat
	}

	return f.localStrat
}

func (f *FNS) GetInfo(ctx context.Context, path string) (*ResourceInfo, error) {
	size, resourceType, modTime, err := f.stratFor(path).GetInfo(ctx, path)
	if err != nil {
		return nil, err
	}

	return &ResourceInfo{
		Path:    path,
		Type:    ResourceType(resourceType),
		Size:    size,
		ModTime: modTime,
	}, nil
}

func (f *FNS) Exists(ctx context.Context, path string) (bool, error) {
	return f.stratFor(path).Exists(ctx, path)
}

func (f *FNS) IsDir(ctx context.Context, path string) (bool, error) {
	return f.stratFor(path).IsDir(ctx, path)
}

func (f *FNS) IsFile(ctx context.Context, path string) (bool, error) {
	return f.stratFor(path).IsFile(ctx, path)
}

func (f *FNS) Read(ctx context.Context, path string) ([]byte, io.ReadCloser, error) {
	if path == "" {
		return nil, nil, errors.Op("Read", path, fmt.Errorf("path cannot be empty"))
	}

	abspath, err := filepath.Abs(path)
	if err != nil {
		abspath = path
	}

	info, err := f.GetInfo(ctx, abspath)
	if err != nil {
		return nil, nil, err
	}

	if info.Type == ResourceTypeDir {
		return nil, nil, errors.Op("Read", path, fmt.Errorf("path is a directory"))
	}

	rc, err := f.stratFor(path).ReadStream(ctx, path)
	if err != nil {
		return nil, nil, err
	}

	if info.Size > f.config.MaxMemorySize {
		return nil, rc, nil
	}

	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, errors.Op("Read", path, err)
	}

	return data, nil, nil
}

func (f *FNS) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	return f.stratFor(path).ReadStream(ctx, path)
}

func (f *FNS) Write(ctx context.Context, path string, data []byte) error {
	return f.stratFor(path).Write(ctx, path, data)
}

func (f *FNS) WriteStream(ctx context.Context, path string, reader io.Reader) error {
	return f.stratFor(path).WriteStream(ctx, path, reader)
}

func (f *FNS) Append(ctx context.Context, path string, data []byte) error {
	return f.stratFor(path).Append(ctx, path, data)
}

func (f *FNS) List(ctx context.Context, path string) ([]ResourceInfo, error) {
	names, err := f.stratFor(path).List(ctx, path)
	if err != nil {
		return nil, err
	}

	var resources []ResourceInfo
	for _, name := range names {
		entryPath := filepath.Join(path, name)
		info, err := f.GetInfo(ctx, entryPath)
		if err != nil {
			continue
		}

		resources = append(resources, *info)
	}

	return resources, nil
}

func (f *FNS) Mkdir(ctx context.Context, path string, perm os.FileMode) error {
	return f.stratFor(path).Mkdir(ctx, path, perm)
}

func (f *FNS) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	return f.stratFor(path).MkdirAll(ctx, path, perm)
}

func (f *FNS) Remove(ctx context.Context, path string) error {
	return f.stratFor(path).Remove(ctx, path)
}

func (f *FNS) RemoveAll(ctx context.Context, path string) error {
	return f.stratFor(path).RemoveAll(ctx, path)
}

func (f *FNS) Copy(ctx context.Context, src, dst string) error {
	if strings.HasPrefix(dst, "http://") || strings.HasPrefix(dst, "https://") {
		return errors.Op("Copy", dst, fmt.Errorf("copying to remote URLs not supported"))
	}

	return f.stratFor(src).Copy(ctx, src, dst)
}

func (f *FNS) Move(ctx context.Context, src, dst string) error {
	return f.stratFor(src).Move(ctx, src, dst)
}

func (f *FNS) Rename(ctx context.Context, src, dst string) error {
	return f.stratFor(src).Rename(ctx, src, dst)
}

func (f *FNS) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	return f.stratFor(path).Chmod(ctx, path, mode)
}

func (f *FNS) Chown(ctx context.Context, path string, uid, gid int) error {
	if runtime.GOOS == "windows" {
		return errors.Op("Chown", path, fmt.Errorf("not supported on Windows"))
	}

	return f.stratFor(path).Chown(ctx, path, uid, gid)
}

func (f *FNS) Download(ctx context.Context, url, dst string, progress func(int)) error {
	if f.stratFor(dst) != f.localStrat {
		return errors.Op("Download", dst, fmt.Errorf("destination must be a local path"))
	}

	return f.stratFor(url).Download(ctx, url, dst, progress)
}

func (f *FNS) DownloadStream(ctx context.Context, url string, progress func(int)) (io.ReadCloser, error) {
	return f.stratFor(url).DownloadStream(ctx, url, progress)
}

func (f *FNS) Fetch(ctx context.Context, url string) ([]byte, error) {
	return f.stratFor(url).Fetch(ctx, url)
}

func (f *FNS) Resolve(ctx context.Context, path string) (string, ResourceType, error) {
	info, err := f.GetInfo(ctx, path)
	if err != nil {
		return "", "", err
	}

	if f.stratFor(path) == f.localStrat {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", "", errors.Op("Resolve", path, err)
		}
		return absPath, info.Type, nil
	}

	return path, info.Type, nil
}

func (f *FNS) Validate(ctx context.Context, path string) error {
	if path == "" {
		return errors.Op("Validate", path, fmt.Errorf("path cannot be empty"))
	}

	return f.stratFor(path).Validate(ctx, path)
}

func (f *FNS) TempFile(ctx context.Context, pattern string) (string, error) {
	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, pattern)
	if err != nil {
		return "", errors.Op("TempFile", pattern, err)
	}

	return tmpFile.Name(), nil
}

func (f *FNS) TempDir(ctx context.Context, pattern string) (string, error) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), pattern)
	if err != nil {
		return "", errors.Op("TempDir", pattern, err)
	}

	return tmpDir, nil
}

func (f *FNS) Walk(ctx context.Context, root string, fn func(path string, info ResourceInfo, err error) error) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		if cbErr := fn(root, ResourceInfo{}, errors.Op("Walk", root, err)); cbErr != nil {
			return cbErr
		}
		return err
	}

	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		if cbErr := fn(rootAbs, ResourceInfo{}, errors.Op("Walk", rootAbs, err)); cbErr != nil {
			return cbErr
		}
		return err
	}

	convertInfo := func(p string) ResourceInfo {
		info, err := f.GetInfo(ctx, p)
		if err != nil {
			return ResourceInfo{}
		}
		return *info
	}

	if cbErr := fn(rootAbs, convertInfo(rootAbs), nil); cbErr != nil {
		return cbErr
	}

	if !rootInfo.IsDir() {
		return nil
	}

	walkFn := func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return fn(p, ResourceInfo{}, errors.Op("Walk", p, err))
		}

		info := convertInfo(p)

		if cbErr := fn(p, info, nil); cbErr != nil {
			return cbErr
		}

		if d.Type()&os.ModeSymlink != 0 {
			targetInfo, err := os.Lstat(p)
			if err == nil && targetInfo.IsDir() {
				return fs.SkipDir
			}
		}
		return nil
	}

	return filepath.WalkDir(rootAbs, walkFn)
}
