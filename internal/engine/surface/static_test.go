package surface_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func get(h http.Handler, method, path string) *httptest.ResponseRecorder {
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
