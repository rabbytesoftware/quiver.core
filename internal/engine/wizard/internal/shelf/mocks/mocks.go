package mocks

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

const (
	NsA        domain.Namespace = "github.com/acme/tool@v1"
	NsA2       domain.Namespace = "github.com/acme/tool@v2"
	NsB        domain.Namespace = "github.com/other/thing@v1"
	BareA      domain.Namespace = "github.com/acme/tool"
	BareB      domain.Namespace = "github.com/other/thing"
	StubTagKey                  = ".stub-owner"
)

type Commander struct {
	Calls   [][]string
	Respond func(name string, args []string) ([]byte, error)
}

func (c *Commander) Run(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	c.Calls = append(c.Calls, append([]string{name}, args...))
	if c.Respond == nil {
		return nil, nil
	}
	return c.Respond(name, args)
}

type Tagger struct {
	ReadErr  error
	WriteErr error
}

func (s *Tagger) Read(
	path string,
) (string, error) {
	if s.ReadErr != nil {
		return "", s.ReadErr
	}
	data, err := os.ReadFile(filepath.Join(path, StubTagKey)) // #nosec G304 -- test double reads paths the test itself created
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

func (s *Tagger) Write(
	path string,
	value string,
) error {
	if s.WriteErr != nil {
		return s.WriteErr
	}
	return os.WriteFile(filepath.Join(path, StubTagKey), []byte(value), 0o600)
}

type UserPath struct {
	mu           sync.Mutex
	Value        string
	ReadErr      error
	WriteErr     error
	BroadcastErr error
	Writes       int
	Broadcasts   int
}

func (u *UserPath) Read() (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.Value, u.ReadErr
}

func (u *UserPath) Write(
	value string,
) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.WriteErr != nil {
		return u.WriteErr
	}
	u.Writes++
	u.Value = value
	return nil
}

func (u *UserPath) Broadcast() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Broadcasts++
	return u.BroadcastErr
}

func (*UserPath) Location() string {
	return `HKCU\Environment\Path`
}

type Sandbox struct {
	Home     string
	UserHome string
	Apps     []string
	NsDir    string
	Bin      string
	Cmd      *Commander
	Tagger   *Tagger
	UserPath *UserPath
	Env      map[string]string
	GOOS     string
	GOARCH   string
}

func NewSandbox(
	t *testing.T,
	goos string,
) *Sandbox {
	t.Helper()
	root := t.TempDir()
	s := &Sandbox{
		Home:     filepath.Join(root, "quiver"),
		UserHome: filepath.Join(root, "user"),
		Apps: []string{
			filepath.Join(root, "Applications"),
			filepath.Join(root, "user", "Applications"),
		},
		Cmd:      &Commander{},
		Tagger:   &Tagger{},
		UserPath: &UserPath{},
		Env:      map[string]string{},
		GOOS:     goos,
		GOARCH:   "amd64",
	}

	nsDir, err := paths.NamespacesAt(s.Home)
	require.NoError(t, err)
	s.NsDir = nsDir

	bin, err := paths.BinAt(s.Home)
	require.NoError(t, err)
	s.Bin = bin

	return s
}

func (s *Sandbox) Host() host.Host {
	h := host.New()
	h.GOARCH = s.GOARCH
	h.HomeDir = s.Home
	h.UserHomeDir = s.UserHome
	h.AppsDirs = s.Apps
	h.Commander = s.Cmd
	h.Env = s.Lookup
	return h
}

func (s *Sandbox) Layout(
	t *testing.T,
) models.Layout {
	t.Helper()
	l, err := s.Host().Layout()
	require.NoError(t, err)
	return l
}

func (s *Sandbox) Lookup(
	key string,
) string {
	return s.Env[key]
}

func (s *Sandbox) Workdir(
	t *testing.T,
	ns domain.Namespace,
) string {
	t.Helper()
	dir := filepath.Join(s.NsDir, filepath.FromSlash(string(ns)))
	require.NoError(t, os.MkdirAll(dir, 0o750))
	return dir
}

func (s *Sandbox) Request(
	t *testing.T,
	ns domain.Namespace,
) (models.Request, string) {
	t.Helper()
	wd := s.Workdir(t, ns)
	return models.Request{Layout: s.Layout(t), Bare: ns.BareNamespace(), Workdir: wd, Moved: map[string]string{}}, wd
}

func WriteFile(
	t *testing.T,
	path string,
	content string,
	mode os.FileMode,
) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chmod(path, mode))
}

func WriteBundle(
	t *testing.T,
	path string,
	version string,
) {
	t.Helper()
	WriteFile(t, filepath.Join(path, "Contents", "version"), version, 0o600)
}

func ReadBundleVersion(
	t *testing.T,
	path string,
) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(path, "Contents", "version")) // #nosec G304 -- test helper reads paths the test itself created
	require.NoError(t, err)
	return string(data)
}

func RequireUnixHost(
	t *testing.T,
) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs symlinks and executable bits of a unix host")
	}
}

func ReadOnlyDir(
	t *testing.T,
) string {
	t.Helper()
	RequireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root writes into read-only directories")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))       // #nosec G302 -- read-only fixture directory
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) }) // #nosec G302 -- restores the fixture directory for cleanup
	return dir
}

func WriteRecord(
	t *testing.T,
	workdir string,
	apps ...domain.PortableApp,
) {
	t.Helper()
	data, err := json.Marshal(domain.PortableRecord{Apps: apps})
	require.NoError(t, err)
	WriteFile(t, filepath.Join(workdir, domain.PortableRecordFile), string(data), 0o644)
}
