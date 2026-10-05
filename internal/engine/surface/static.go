package surface

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
)

// newStatic serves dir read-only. The directory is opened as an os.Root per
// request, so a symlink pointing outside dir cannot be followed.
func newStatic(
	dir string,
) (http.Handler, error) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("surface: static dir %q is not a directory", dir)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			http.Error(w, "surface unavailable", http.StatusServiceUnavailable)
			return
		}
		defer func() { _ = root.Close() }()
		serveStatic(w, r, root)
	}), nil
}

func serveStatic(
	w http.ResponseWriter,
	r *http.Request,
	root *os.Root,
) {
	name := path.Clean("/" + r.URL.Path)[1:]
	if name == "" {
		name = "index.html"
	}

	f, name, err := openServable(root, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, info.ModTime(), io.ReadSeeker(f))
}

// openServable opens name, descending into a directory's index.html. A missing
// extensionless path falls back to the root index.html (SPA routing).
func openServable(
	root *os.Root,
	name string,
) (*os.File, string, error) {
	f, err := root.Open(name)
	if err != nil {
		return spaFallback(root, name, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, name, err
	}
	if !info.IsDir() {
		return f, name, nil
	}
	_ = f.Close()
	name = path.Join(name, "index.html")
	if f, err = root.Open(name); err != nil {
		return spaFallback(root, name, err)
	}
	return f, name, nil
}

func spaFallback(
	root *os.Root,
	name string,
	err error,
) (*os.File, string, error) {
	if path.Ext(name) != "" {
		return nil, name, err
	}
	f, err := root.Open("index.html")
	return f, "index.html", err
}
