//go:build e2e

// Command githost serves every bare repository under a root directory over
// git's smart HTTP protocol, behind TLS, by handing each request to
// git-http-backend. It exists only for the docker end-to-end suite: the
// daemon's resolver clones https://<host>/<user>/<repo> for a host it does
// not know, so a container-local HTTPS git server is the one route to a
// repository whose tags the suite can move.
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/cgi" //nolint:gosec // G504 targets Go < 1.6.3; this toolchain is 1.26 and the handler serves only a test container
	"time"
)

func main() {
	addr := flag.String("addr", ":443", "listen address")
	root := flag.String("root", "/srv/git", "directory holding the bare repositories")
	cert := flag.String("cert", "", "TLS certificate (PEM)")
	key := flag.String("key", "", "TLS private key (PEM)")
	backend := flag.String("backend", "/usr/lib/git-core/git-http-backend", "git-http-backend binary")
	flag.Parse()

	handler := &cgi.Handler{
		Path: *backend,
		Env: []string{
			"GIT_PROJECT_ROOT=" + *root,
			"GIT_HTTP_EXPORT_ALL=1",
		},
	}

	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("githost: serving %s on %s", *root, *addr)
	log.Fatal(server.ListenAndServeTLS(*cert, *key))
}
