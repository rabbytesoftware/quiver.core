package surface_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
)

func staticSite(t *testing.T) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("INDEX"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "a.js"), []byte("JS"), 0o644))
	h, err := surface.New(t.TempDir()).Handler(surface.Spec{Mode: domainRuntime.SurfaceModeStatic, Dir: dir})
	require.NoError(t, err)
	return h, dir
}

func get(
	h http.Handler,
	method string,
	path string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestStatic_ServesFilesAndIndex(t *testing.T) {
	h, _ := staticSite(t)
	require.Equal(t, "INDEX", get(h, http.MethodGet, "/").Body.String())
	require.Equal(t, "JS", get(h, http.MethodGet, "/assets/a.js").Body.String())
}

func TestStatic_SPAFallbackOnlyForExtensionless(t *testing.T) {
	h, _ := staticSite(t)
	require.Equal(t, "INDEX", get(h, http.MethodGet, "/some/route").Body.String())
	require.Equal(t, http.StatusNotFound, get(h, http.MethodGet, "/missing.js").Code)
}

func TestStatic_ReadOnlyMethods(t *testing.T) {
	h, _ := staticSite(t)
	require.Equal(t, http.StatusOK, get(h, http.MethodHead, "/").Code)
	require.Equal(t, http.StatusMethodNotAllowed, get(h, http.MethodPost, "/").Code)
}

func TestStatic_RefusesTraversalAndSymlinkEscape(t *testing.T) {
	h, dir := staticSite(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("SECRET"), 0o644))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, "leak.txt")))

	require.NotContains(t, get(h, http.MethodGet, "/../../etc/passwd").Body.String(), "root:")
	require.NotContains(t, get(h, http.MethodGet, "/leak.txt").Body.String(), "SECRET")
}

func TestStatic_MissingDirIsAnError(t *testing.T) {
	_, err := surface.New(t.TempDir()).Handler(surface.Spec{Mode: domainRuntime.SurfaceModeStatic, Dir: "/nonexistent-dir-xyz"})
	require.Error(t, err)
}

func TestStatic_DirectoryServesItsIndex(t *testing.T) {
	h, dir := staticSite(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "index.html"), []byte("DOCS"), 0o644))

	require.Equal(t, "DOCS", get(h, http.MethodGet, "/docs").Body.String())
	require.Equal(t, http.StatusNotFound, get(h, http.MethodGet, "/assets").Code, "a directory without an index is not listed")
}

func TestStatic_NoIndexIs404(t *testing.T) {
	dir := t.TempDir()
	h, err := surface.New(t.TempDir()).Handler(surface.Spec{Mode: domainRuntime.SurfaceModeStatic, Dir: dir})
	require.NoError(t, err)

	require.Equal(t, http.StatusNotFound, get(h, http.MethodGet, "/").Code)
}

func TestStatic_DirRemovedAfterStartIs503(t *testing.T) {
	dir := t.TempDir()
	h, err := surface.New(t.TempDir()).Handler(surface.Spec{Mode: domainRuntime.SurfaceModeStatic, Dir: dir})
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(dir))

	require.Equal(t, http.StatusServiceUnavailable, get(h, http.MethodGet, "/").Code)
}

// linkOrSkip makes a symlink, skipping the test where the platform will not
// let this user create one (windows without the privilege or developer mode).
func linkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
}

// What nothing outside the served dir may ever reveal, whatever spelling the
// request takes: links of every kind pointing out, parent segments written
// every way, drive and UNC paths, alternate streams and device names.
func TestStatic_NeverServesOutsideTheDir(t *testing.T) {
	h, dir := staticSite(t)
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "deep"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("SECRETDATA"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "deep", "inner.txt"), []byte("SECRETDATA"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(dir), "sibling.txt"), []byte("SECRETDATA"), 0o644))

	linkOrSkip(t, filepath.Join(outside, "secret.txt"), filepath.Join(dir, "file_link.txt"))
	linkOrSkip(t, outside, filepath.Join(dir, "dir_link"))
	linkOrSkip(t, filepath.Join("..", filepath.Base(outside)), filepath.Join(dir, "rel_link"))
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(dir, "junction"), outside).CombinedOutput()
		require.NoError(t, err, string(out))
	}

	paths := []string{
		"/file_link.txt", "/dir_link/secret.txt", "/dir_link/deep/inner.txt", "/dir_link/", "/rel_link/secret.txt",
		"/junction/secret.txt", "/junction/deep/inner.txt", "/FILE_LINK.TXT", "/Dir_Link/secret.txt",
		"/../sibling.txt", "/%2e%2e/sibling.txt", "/%2E%2E/%2E%2E/sibling.txt", "/..%5Csibling.txt",
		"/..\\sibling.txt", "/sub\\..\\..\\sibling.txt", "/%2e%2e%5csibling.txt",
		"/" + filepath.ToSlash(filepath.Join(outside, "secret.txt")), "/C:/secret.txt", "/C%3A/secret.txt",
		"//localhost/c$/secret.txt", "/%5C%5Clocalhost%5Cc$%5Csecret.txt", "/%5C%5C%3F%5C" + filepath.Base(outside),
		"/index.html::$DATA", "/index.html:stream", "/assets::$INDEX_ALLOCATION/a.js",
		"/CON", "/NUL", "/aux.html", "/COM1", "/con.html", "/CONIN$", "/CONOUT$",
		"/index.html.", "/index.html%20", "/INDEX.HTML", "/index.html%00.txt",
	}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, "http://x"+p, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.NotContains(t, rec.Body.String(), "SECRETDATA", "request %q leaked a file outside the dir", p)
	}
}
