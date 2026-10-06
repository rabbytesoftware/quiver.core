// Package surface owns how an arrow's interface is reached: the unix socket
// address handed to the arrow, readiness probing, and the handlers that serve
// a surface. It opens no TCP listener.
package surface

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

var (
	// ErrPathTooLong means the socket address would not fit a sockaddr_un.
	ErrPathTooLong = errors.New("surface: socket path too long")
	// ErrNoSurface means the spec names no servable surface.
	ErrNoSurface = errors.New("surface: no servable surface")
)

// Spec identifies the surface to serve.
type Spec struct {
	Mode      domainRuntime.SurfaceMode
	Namespace domain.Namespace
	// Dir is the absolute directory served in static mode.
	Dir string
}

// Surface provisions and serves arrow interfaces.
type Surface interface {
	// SocketPath is the deterministic address for ns. It touches nothing.
	SocketPath(
		ns domain.Namespace,
	) string
	// Prepare makes the run directory private and clears a stale socket for
	// ns, returning the address. A socket something still answers on is left
	// alone, so a rejected duplicate execute cannot break a live arrow.
	Prepare(
		ns domain.Namespace,
	) (string, error)
	// Cleanup removes ns's socket file.
	Cleanup(
		ns domain.Namespace,
	)
	// Handler serves the surface described by spec.
	Handler(
		spec Spec,
	) (http.Handler, error)
	// Ready reports whether the surface answers a request for path.
	Ready(
		ctx context.Context,
		spec Spec,
		path string,
	) bool
}

type cachedProxy struct {
	handler   http.Handler
	transport *http.Transport
}

type engine struct {
	runDir  string
	mu      sync.Mutex
	proxies map[string]cachedProxy
}

// New builds a Surface that keeps its sockets under runDir.
func New(
	runDir string,
) Surface {
	return &engine{runDir: runDir, proxies: make(map[string]cachedProxy)}
}

func (e *engine) SocketPath(
	ns domain.Namespace,
) string {
	sum := sha256.Sum256([]byte(ns.String()))
	return filepath.Join(e.runDir, hex.EncodeToString(sum[:6])+".sock")
}

func (e *engine) Prepare(
	ns domain.Namespace,
) (string, error) {
	path := e.SocketPath(ns)
	if len(path) > MaxSocketPath {
		return "", fmt.Errorf("%w: %d bytes (max %d): %s", ErrPathTooLong, len(path), MaxSocketPath, path)
	}
	if err := os.MkdirAll(e.runDir, 0o700); err != nil {
		return "", fmt.Errorf("surface: create run dir: %w", err)
	}
	if err := os.Chmod(e.runDir, 0o700); err != nil { // #nosec G302 -- tightens a directory to owner-only; it needs its execute bit
		return "", fmt.Errorf("surface: secure run dir: %w", err)
	}

	if conn, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		return path, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("surface: clear stale socket: %w", err)
	}
	return path, nil
}

func (e *engine) Cleanup(
	ns domain.Namespace,
) {
	path := e.SocketPath(ns)
	e.mu.Lock()
	cached, ok := e.proxies[path]
	delete(e.proxies, path)
	e.mu.Unlock()
	if ok {
		cached.transport.CloseIdleConnections()
	}
	_ = os.Remove(path)
}

// proxy returns the one proxy for socket, so every request shares a single
// bounded connection pool.
func (e *engine) proxy(
	socket string,
) http.Handler {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cached, ok := e.proxies[socket]; ok {
		return cached.handler
	}
	handler, transport := newProxy(socket)
	e.proxies[socket] = cachedProxy{handler: handler, transport: transport}
	return handler
}

func (e *engine) Handler(
	spec Spec,
) (http.Handler, error) {
	switch spec.Mode {
	case domainRuntime.SurfaceModeListen:
		return e.proxy(e.SocketPath(spec.Namespace)), nil
	case domainRuntime.SurfaceModeStatic:
		return newStatic(spec.Dir)
	default:
		return nil, ErrNoSurface
	}
}

func (e *engine) Ready(
	ctx context.Context,
	spec Spec,
	path string,
) bool {
	switch spec.Mode {
	case domainRuntime.SurfaceModeStatic:
		return true
	case domainRuntime.SurfaceModeListen:
		return probe(ctx, e.SocketPath(spec.Namespace), path)
	default:
		return false
	}
}
