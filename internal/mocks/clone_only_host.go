package mocks

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

// CloneOnlyHost is a repository reachable only by cloning, the way a
// self-hosted git server is: it serves a manifest at a tag or branch name and
// refuses anything else, a commit SHA included. It answers both resolver
// interfaces, so manifold.NewWithResolvers(host, host) is a real manifold
// over it.
type CloneOnlyHost struct {
	// Manifests maps a tag or branch name to the ARROW.md served there.
	Manifests map[string][]byte
	// Snapshot is the ref advertisement the host answers.
	Snapshot domain.RefSnapshot

	mu       sync.Mutex
	requests []domain.Namespace
}

// ReleaseManifest is a valid arrow@v0 manifest named name, runnable on every
// platform.
func ReleaseManifest(
	name string,
) []byte {
	return []byte(`schema: "arrow@v0"
metadata:
  name: "` + name + `"
  description: "a release served by a clone-only host"
targets:
  "*":
    lifecycle:
      install:
        - type: run
          title: Install
          command: echo install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          title: Uninstall
          command: echo uninstall
          timeout: 10s
          exit_on_failure: false
`)
}

// Manifold is a real manifold resolving every namespace through h.
func (h *CloneOnlyHost) Manifold() manifold.Manifold {
	return manifold.NewWithResolvers(h, h, nil)
}

// Requests lists every manifest fetch h answered or refused, in order.
func (h *CloneOnlyHost) Requests() []domain.Namespace {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]domain.Namespace(nil), h.requests...)
}

func (h *CloneOnlyHost) ResolveArrow(
	_ context.Context,
	ns domain.Namespace,
) ([]byte, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, ns)
	data, ok := h.Manifests[ns.Ref()]
	if !ok {
		return nil, "", resolver.ErrFetchFailed
	}
	return data, "ARROW.md", nil
}

func (h *CloneOnlyHost) ResolveArrowAt(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]byte, string, error) {
	return nil, "", resolver.ErrFetchFailed
}

func (h *CloneOnlyHost) ResolveCollection(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, error) {
	return nil, resolver.ErrFetchFailed
}

func (h *CloneOnlyHost) Refs(
	_ context.Context,
	_ domain.Namespace,
) (domain.RefSnapshot, error) {
	return h.Snapshot, nil
}
