//go:build e2e

// Command githost serves every bare repository under a root directory over
// git's smart HTTP protocol, behind TLS, by handing each request to
// git-http-backend. It exists only for the docker end-to-end suite: the
// daemon's resolver clones https://<host>/<user>/<repo> for a host it does
// not know, so a container-local HTTPS git server is the one route to a
// repository whose tags the suite can move.
//
// The self-update phases also point github.com and raw.githubusercontent.com
// at this server, so it answers the two other GitHub shapes quiver.core's own
// row reaches: a raw file at a ref (raw.githubusercontent.com/<user>/<repo>/
// <ref>/<file>, read from the bare repository with git show) and a release
// asset (github.com/<user>/<repo>/releases/download/<tag>/<name>, read from
// the releases directory).
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/cgi" //nolint:gosec // G504 targets Go < 1.6.3; this toolchain is 1.26 and the handler serves only a test container
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	rawHost    = "raw.githubusercontent.com"
	githubHost = "github.com"
)

func main() {
	addr := flag.String("addr", ":443", "listen address")
	root := flag.String("root", "/srv/git", "directory holding the bare repositories")
	releases := flag.String("releases", "/srv/releases", "directory holding release assets as <user>/<repo>/<tag>/<name>")
	cert := flag.String("cert", "", "TLS certificate (PEM)")
	key := flag.String("key", "", "TLS private key (PEM)")
	backend := flag.String("backend", "/usr/lib/git-core/git-http-backend", "git-http-backend binary")
	flag.Parse()

	git := &cgi.Handler{
		Path: *backend,
		Env: []string{
			"GIT_PROJECT_ROOT=" + *root,
			"GIT_HTTP_EXPORT_ALL=1",
		},
	}

	server := &http.Server{
		Addr:              *addr,
		Handler:           route(*root, *releases, git),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("githost: serving %s on %s", *root, *addr)
	log.Fatal(server.ListenAndServeTLS(*cert, *key))
}

func route(
	root string,
	releases string,
	git http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		switch {
		case host == rawHost:
			serveRaw(w, r, root)
		case host == githubHost && strings.Contains(r.URL.Path, "/releases/download/"):
			serveAsset(w, r, releases)
		default:
			git.ServeHTTP(w, r)
		}
	})
}

// serveRaw answers /<user>/<repo>/<ref>/<file> with the file's content at ref.
func serveRaw(
	w http.ResponseWriter,
	r *http.Request,
	root string,
) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 4)
	if len(parts) != 4 || !safe(parts[:3]...) || strings.Contains(parts[3], "..") {
		http.NotFound(w, r)
		return
	}
	repo := filepath.Join(root, parts[0], parts[1])
	out, err := exec.CommandContext(r.Context(), "git", "--git-dir", repo, "show", parts[2]+":"+parts[3]).Output() // #nosec -- every argument is validated above and passed without a shell
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(out)
}

// serveAsset answers /<user>/<repo>/releases/download/<tag>/<name>.
func serveAsset(
	w http.ResponseWriter,
	r *http.Request,
	releases string,
) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" || !safe(parts[0], parts[1], parts[4], parts[5]) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(releases, parts[0], parts[1], parts[4], parts[5]))
}

func safe(
	segments ...string,
) bool {
	for _, s := range segments {
		if s == "" || s == "." || s == ".." || strings.HasPrefix(s, "-") || strings.ContainsAny(s, `\:`) {
			return false
		}
	}
	return true
}
