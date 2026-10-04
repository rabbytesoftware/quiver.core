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
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// maxSocketPath is below the darwin limit (104 with the NUL) and the linux
// one (108).
const maxSocketPath = 100

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
	SocketPath(ns domain.Namespace) string
	// Prepare makes the run directory private and clears a stale socket for
	// ns, returning the address. A socket something still answers on is left
	// alone, so a rejected duplicate execute cannot break a live arrow.
	Prepare(ns domain.Namespace) (string, error)
	// Cleanup removes ns's socket file.
	Cleanup(ns domain.Namespace)
	// Handler serves the surface described by spec.
	Handler(spec Spec) (http.Handler, error)
	// Ready reports whether the surface answers a request for path.
	Ready(ctx context.Context, spec Spec, path string) bool
}

type engine struct{ runDir string }

// New builds a Surface that keeps its sockets under runDir.
func New(runDir string) Surface { return &engine{runDir: runDir} }

func (e *engine) SocketPath(ns domain.Namespace) string {
	sum := sha256.Sum256([]byte(ns.String()))
	return filepath.Join(e.runDir, hex.EncodeToString(sum[:6])+".sock")
}

func (e *engine) Prepare(ns domain.Namespace) (string, error) {
	path := e.SocketPath(ns)
	if len(path) > maxSocketPath {
		return "", fmt.Errorf("%w: %d bytes (max %d): %s", ErrPathTooLong, len(path), maxSocketPath, path)
	}
	if err := os.MkdirAll(e.runDir, 0o700); err != nil {
		return "", fmt.Errorf("surface: create run dir: %w", err)
	}
	//nolint:gosec // a directory needs its execute bit; 0700 is owner-only
	if err := os.Chmod(e.runDir, 0o700); err != nil {
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

func (e *engine) Cleanup(ns domain.Namespace) {
	_ = os.Remove(e.SocketPath(ns))
}

func (e *engine) Handler(spec Spec) (http.Handler, error) {
	switch spec.Mode {
	case domainRuntime.SurfaceModeListen:
		return newProxy(e.SocketPath(spec.Namespace)), nil
	case domainRuntime.SurfaceModeStatic:
		return newStatic(spec.Dir)
	default:
		return nil, ErrNoSurface
	}
}

func (e *engine) Ready(ctx context.Context, spec Spec, path string) bool {
	switch spec.Mode {
	case domainRuntime.SurfaceModeStatic:
		return true
	case domainRuntime.SurfaceModeListen:
		return probe(ctx, e.SocketPath(spec.Namespace), path)
	default:
		return false
	}
}
