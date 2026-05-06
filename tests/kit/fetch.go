//go:build integration

package kit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ServeFetch starts a local HTTP file server serving content at /file.
// Registers t.Cleanup(srv.Close). Returns the full URL (http://127.0.0.1:<port>/file).
func ServeFetch(
	t *testing.T,
	content []byte,
) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/file"
}

// RenderFixture reads a fixture file by relative path under testdata/arrows/,
// replaces all {{KEY}} tokens with values from vars, and returns the rendered bytes.
func RenderFixture(
	t *testing.T,
	path string,
	vars map[string]string,
) []byte {
	t.Helper()
	content := ReadFixture(t, path)
	s := string(content)
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return []byte(s)
}
