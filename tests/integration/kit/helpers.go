//go:build integration

package kit

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	dto "github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// NSFor constructs a versioned test namespace.
// fixture: relative path under testdata/arrows/ e.g. "quiver-test/tool-a"
// tag: git tag e.g. "v1"
// Returns: "quiver.test/quiver-test/tool-a@v1"
func NSFor(fixture, tag string) string {
	return "quiver.test/" + fixture + "@" + tag
}

// NSForGlob constructs a glob-constrained namespace.
// glob: constraint pattern e.g. "v*"
func NSForGlob(fixture, glob string) string {
	return "quiver.test/" + fixture + "@" + glob
}

// BuildMinimalYAML produces a minimal valid arrow manifest with the given name.
func BuildMinimalYAML(name string) []byte {
	return []byte(fmt.Sprintf(`schema: "arrow@v0"
metadata:
  name: %s
  version: 1.0.0
  description: test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`, name))
}

// WaitForState polls GetDetail until the arrow reaches want state or timeout elapses.
func WaitForState(
	t *testing.T,
	tc *TypedClient,
	ns string,
	want domain.ArrowState,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last domain.ArrowState
	for time.Now().Before(deadline) {
		detail, status := tc.GetDetail(ns)
		if status == http.StatusOK {
			last = domain.ArrowState(detail.State)
			if last == want {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("WaitForState(%s): timeout waiting for %s, last=%s", ns, want, last)
}

// WaitForListLen polls List until it returns exactly wantLen items or timeout elapses.
func WaitForListLen(
	t *testing.T,
	tc *TypedClient,
	wantLen int,
	timeout time.Duration,
) []dto.ArrowListItemDTO {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		items, status := tc.List()
		if status == http.StatusOK && len(items) == wantLen {
			return items
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("WaitForListLen: timeout waiting for %d items in list", wantLen)
	return nil
}
