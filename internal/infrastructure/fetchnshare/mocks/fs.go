package mocks

import (
	"io"
	"io/fs"
	"os"
	"time"
)

type FileSystem interface {
	Open(name string) (File, error)
	Create(name string) (File, error)
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
	Stat(name string) (os.FileInfo, error)
	Lstat(name string) (os.FileInfo, error)
	Remove(name string) error
	RemoveAll(path string) error
	Rename(oldpath, newpath string) error
	Mkdir(name string, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	ReadDir(dirname string) ([]os.DirEntry, error)
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
	Chmod(name string, mode os.FileMode) error
	Chown(name string, uid, gid int) error
}

type File interface {
	io.ReadWriteCloser
	Stat() (os.FileInfo, error)
	Sync() error
}

type RealFileSystem struct{}

func (RealFileSystem) Open(name string) (File, error) {
	return os.Open(name)
}

func (RealFileSystem) Create(name string) (File, error) {
	return os.Create(name)
}

func (RealFileSystem) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	return os.OpenFile(name, flag, perm)
}

func (RealFileSystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (RealFileSystem) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (RealFileSystem) Remove(name string) error {
	return os.Remove(name)
}

func (RealFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (RealFileSystem) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (RealFileSystem) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm)
}

func (RealFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (RealFileSystem) ReadDir(dirname string) ([]os.DirEntry, error) {
	return os.ReadDir(dirname)
}

func (RealFileSystem) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (RealFileSystem) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (RealFileSystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (RealFileSystem) Chown(name string, uid, gid int) error {
	return os.Chown(name, uid, gid)
}

type MockFileSystem struct {
	OpenFunc      func(name string) (File, error)
	CreateFunc    func(name string) (File, error)
	OpenFileFunc  func(name string, flag int, perm os.FileMode) (File, error)
	StatFunc      func(name string) (os.FileInfo, error)
	LstatFunc     func(name string) (os.FileInfo, error)
	RemoveFunc    func(name string) error
	RemoveAllFunc func(path string) error
	RenameFunc    func(oldpath, newpath string) error
	MkdirFunc     func(name string, perm os.FileMode) error
	MkdirAllFunc  func(path string, perm os.FileMode) error
	ReadDirFunc   func(dirname string) ([]os.DirEntry, error)
	ReadFileFunc  func(filename string) ([]byte, error)
	WriteFileFunc func(filename string, data []byte, perm os.FileMode) error
	ChmodFunc     func(name string, mode os.FileMode) error
	ChownFunc     func(name string, uid, gid int) error
}

func (m *MockFileSystem) Open(name string) (File, error) {
	if m.OpenFunc != nil {
		return m.OpenFunc(name)
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) Create(name string) (File, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(name)
	}
	return nil, os.ErrPermission
}

func (m *MockFileSystem) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	if m.OpenFileFunc != nil {
		return m.OpenFileFunc(name, flag, perm)
	}
	return nil, os.ErrPermission
}

func (m *MockFileSystem) Stat(name string) (os.FileInfo, error) {
	if m.StatFunc != nil {
		return m.StatFunc(name)
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) Lstat(name string) (os.FileInfo, error) {
	if m.LstatFunc != nil {
		return m.LstatFunc(name)
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) Remove(name string) error {
	if m.RemoveFunc != nil {
		return m.RemoveFunc(name)
	}
	return os.ErrPermission
}

func (m *MockFileSystem) RemoveAll(path string) error {
	if m.RemoveAllFunc != nil {
		return m.RemoveAllFunc(path)
	}
	return os.ErrPermission
}

func (m *MockFileSystem) Rename(oldpath, newpath string) error {
	if m.RenameFunc != nil {
		return m.RenameFunc(oldpath, newpath)
	}
	return os.ErrPermission
}

func (m *MockFileSystem) Mkdir(name string, perm os.FileMode) error {
	if m.MkdirFunc != nil {
		return m.MkdirFunc(name, perm)
	}
	return os.ErrPermission
}

func (m *MockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	if m.MkdirAllFunc != nil {
		return m.MkdirAllFunc(path, perm)
	}
	return os.ErrPermission
}

func (m *MockFileSystem) ReadDir(dirname string) ([]os.DirEntry, error) {
	if m.ReadDirFunc != nil {
		return m.ReadDirFunc(dirname)
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) ReadFile(filename string) ([]byte, error) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(filename)
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) WriteFile(filename string, data []byte, perm os.FileMode) error {
	if m.WriteFileFunc != nil {
		return m.WriteFileFunc(filename, data, perm)
	}
	return os.ErrPermission
}

func (m *MockFileSystem) Chmod(name string, mode os.FileMode) error {
	if m.ChmodFunc != nil {
		return m.ChmodFunc(name, mode)
	}
	return os.ErrPermission
}

func (m *MockFileSystem) Chown(name string, uid, gid int) error {
	if m.ChownFunc != nil {
		return m.ChownFunc(name, uid, gid)
	}
	return os.ErrPermission
}

type MockFileInfo struct {
	NameFunc    func() string
	SizeFunc    func() int64
	ModeFunc    func() os.FileMode
	ModTimeFunc func() time.Time
	IsDirFunc   func() bool
	SysFunc     func() interface{}
}

func (m *MockFileInfo) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-file"
}

func (m *MockFileInfo) Size() int64 {
	if m.SizeFunc != nil {
		return m.SizeFunc()
	}
	return 0
}

func (m *MockFileInfo) Mode() os.FileMode {
	if m.ModeFunc != nil {
		return m.ModeFunc()
	}
	return 0644
}

func (m *MockFileInfo) ModTime() time.Time {
	if m.ModTimeFunc != nil {
		return m.ModTimeFunc()
	}
	return time.Now()
}

func (m *MockFileInfo) IsDir() bool {
	if m.IsDirFunc != nil {
		return m.IsDirFunc()
	}
	return false
}

func (m *MockFileInfo) Sys() interface{} {
	if m.SysFunc != nil {
		return m.SysFunc()
	}
	return nil
}

type MockDirEntry struct {
	NameFunc  func() string
	IsDirFunc func() bool
	TypeFunc  func() fs.FileMode
	InfoFunc  func() (fs.FileInfo, error)
}

func (m *MockDirEntry) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-entry"
}

func (m *MockDirEntry) IsDir() bool {
	if m.IsDirFunc != nil {
		return m.IsDirFunc()
	}
	return false
}

func (m *MockDirEntry) Type() fs.FileMode {
	if m.TypeFunc != nil {
		return m.TypeFunc()
	}
	return 0
}

func (m *MockDirEntry) Info() (fs.FileInfo, error) {
	if m.InfoFunc != nil {
		return m.InfoFunc()
	}
	return &MockFileInfo{}, nil
}
