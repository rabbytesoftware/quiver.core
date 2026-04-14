# Paths Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce `internal/core/paths` — a thin module that resolves a named Quiver directory, creates it under a mutex if absent, and returns its absolute path; replace all scattered `metadata.GetXxxPath()` + `os.MkdirAll` pairs across the codebase with a single call.

**Architecture:** A package-private `ensure(path string) (string, error)` function uses a `sync.Map` of per-path `sync.Mutex` values to serialize directory creation. Four named public functions (`Events`, `Store`, `Namespaces`) call `ensure` with the path from `metadata`. Call sites in `adapter`, `engine`, and `app` layers swap their two-step pattern for a single call. The `metadata` package is unchanged — it remains the single source of path strings; `paths` adds lifecycle (mkdir + mutex) on top.

**Tech Stack:** Go standard library (`os`, `sync`), `github.com/rabbytesoftware/quiver/internal/core/metadata`

---

## File Map

| Action | File |
|---|---|
| **Create** | `internal/core/paths/paths.go` |
| **Create** | `internal/core/paths/paths_test.go` |
| **Modify** | `internal/adapter/container.go` |
| **Modify** | `internal/engine/container.go` |
| **Modify** | `internal/app/arrow/builder.go` |
| **Modify** | `internal/app/quiver/builder.go` |

---

### Task 1: `internal/core/paths` package

**Files:**
- Create: `internal/core/paths/paths.go`
- Create: `internal/core/paths/paths_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/core/paths/paths_test.go
package paths_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ensureCreatesDir verifies that calling fn creates the directory on disk.
func ensureCreatesDir(t *testing.T, fn func() (string, error)) {
	t.Helper()
	got, err := fn()
	require.NoError(t, err)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr, "directory should exist after call")
	assert.True(t, info.IsDir())
}

func TestEvents_CreatesDir(t *testing.T) {
	ensureCreatesDir(t, paths.Events)
}

func TestEvents_ReturnsAbsolutePath(t *testing.T) {
	got, err := paths.Events()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}

func TestStore_CreatesDir(t *testing.T) {
	ensureCreatesDir(t, paths.Store)
}

func TestStore_ReturnsAbsolutePath(t *testing.T) {
	got, err := paths.Store()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}

func TestNamespaces_CreatesDir(t *testing.T) {
	ensureCreatesDir(t, paths.Namespaces)
}

func TestNamespaces_ReturnsAbsolutePath(t *testing.T) {
	got, err := paths.Namespaces()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}

func TestEvents_Idempotent(t *testing.T) {
	first, err := paths.Events()
	require.NoError(t, err)
	second, err := paths.Events()
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestConcurrentCalls_NoRace(t *testing.T) {
	// Run with -race to detect data races.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = paths.Events()
			_, _ = paths.Store()
			_, _ = paths.Namespaces()
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/char2cs/.superset/worktrees/quiver.core/fix/quiver-home-organization
go test ./internal/core/paths/... -v
```

Expected: compilation failure — package `paths` does not exist.

- [ ] **Step 3: Implement `paths.go`**

```go
// internal/core/paths/paths.go
package paths

import (
	"fmt"
	"os"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/metadata"
)

// mu stores one *sync.Mutex per absolute path, created on first access.
// Serializes concurrent directory creation for the same path.
var mu sync.Map

// ensure creates dir at path if it does not exist and returns path.
// Concurrent calls for the same path are serialized by a per-path mutex.
func ensure(path string) (string, error) {
	v, _ := mu.LoadOrStore(path, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	defer m.Unlock()
	if err := os.MkdirAll(path, 0750); err != nil {
		return "", fmt.Errorf("paths: create %q: %w", path, err)
	}
	return path, nil
}

// Events returns the absolute path to the event-store directory,
// creating it if it does not exist.
func Events() (string, error) {
	return ensure(metadata.GetEventsPath())
}

// Store returns the absolute path to the catalog read-model directory,
// creating it if it does not exist.
func Store() (string, error) {
	return ensure(metadata.GetStorePath())
}

// Namespaces returns the absolute path to the namespaces directory,
// creating it if it does not exist.
func Namespaces() (string, error) {
	return ensure(metadata.GetNamespacesPath())
}
```

- [ ] **Step 4: Run tests to confirm they pass (including race detector)**

```bash
go test ./internal/core/paths/... -v -race
```

Expected: all 8 tests PASS, no race conditions reported.

- [ ] **Step 5: Commit**

```bash
git add internal/core/paths/paths.go internal/core/paths/paths_test.go
git commit -m "feat(core/paths): add paths module — ensure-and-return directory helpers"
```

---

### Task 2: Update all call sites

**Files:**
- Modify: `internal/adapter/container.go`
- Modify: `internal/engine/container.go`
- Modify: `internal/app/arrow/builder.go`
- Modify: `internal/app/quiver/builder.go`

- [ ] **Step 1: Update `internal/adapter/container.go`**

Replace the `metadata.GetEventsPath()` + `os.MkdirAll` block with a single `paths.Events()` call. Remove the `"os"` import (no longer needed). Replace the entire file:

```go
package adapter

import (
	"fmt"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
)

type Container struct {
	ArrowES   asynxModels.Store
	RuntimeES asynxModels.Store
	QuiverES  asynxModels.Store
}

func Init() (*Container, error) {
	eventsPath, err := paths.Events()
	if err != nil {
		return nil, fmt.Errorf("adapter: %w", err)
	}

	arrowES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "arrow.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: arrow event store: %w", err)
	}

	runtimeES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "runtime.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: runtime event store: %w", err)
	}

	quiverES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "quiver.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: quiver event store: %w", err)
	}

	return &Container{
		ArrowES:   arrowES,
		RuntimeES: runtimeES,
		QuiverES:  quiverES,
	}, nil
}
```

- [ ] **Step 2: Update `internal/engine/container.go`**

Replace the `metadata.GetEventsPath()` + `os.MkdirAll` block with `paths.Events()`. Remove `"os"` and `metadata` imports. Replace the entire file:

```go
package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

const manifoldFetchTimeout = 30 * time.Second

// Container holds all engine-layer dependencies.
type Container struct {
	Vault     vault.Vault
	Manifold  manifold.Manifold
	Wizard    wizard.Wizard
	Netbridge netbridge.Netbridge
	DepTree   deptree.DepTree
}

// Init constructs all engines and returns a ready-to-use Container.
func Init(ctx context.Context) (*Container, error) {
	eventsPath, err := paths.Events()
	if err != nil {
		return nil, fmt.Errorf("engine container: %w", err)
	}

	es, err := sqlite.NewEventStore(filepath.Join(eventsPath, "netbridge.db"))
	if err != nil {
		return nil, fmt.Errorf("engine container: %w", err)
	}

	nb, err := netbridge.New().WithEventStore(es).Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("engine container: netbridge: %w", err)
	}

	wiz, err := wizard.New()
	if err != nil {
		return nil, fmt.Errorf("engine container: wizard: %w", err)
	}

	return &Container{
		Vault:     vault.New("", 0, domain.CurrentOS()),
		Manifold:  manifold.New(manifoldFetchTimeout),
		Wizard:    wiz,
		Netbridge: nb,
		DepTree:   deptree.New(),
	}, nil
}
```

- [ ] **Step 3: Update `internal/app/arrow/builder.go`**

Replace the `storePath := metadata.GetStorePath()` + `os.MkdirAll` block (lines 110–113) with a single `paths.Store()` call. Remove `"os"` and `metadata` imports if they are no longer used elsewhere in the file. Only the catalog-creation block changes:

```go
cat := b.catalog
if cat == nil {
	storePath, storePathErr := paths.Store()
	if storePathErr != nil {
		return nil, fmt.Errorf("arrow builder: %w", storePathErr)
	}
	store, storeErr := arrowstore.NewArrowCatalog(filepath.Join(storePath, "arrows.db"))
	if storeErr != nil {
		return nil, storeErr
	}
	cat, err = catalog.New(axArrow, axRuntime, store, e.Vault, e.Manifold)
	if err != nil {
		return nil, err
	}
}
```

Add `"github.com/rabbytesoftware/quiver/internal/core/paths"` to imports. Remove `"github.com/rabbytesoftware/quiver/internal/core/metadata"` and `"os"` if they are no longer referenced anywhere else in `builder.go`.

- [ ] **Step 4: Update `internal/app/quiver/builder.go`**

Same pattern as arrow builder. Replace the `storePath := metadata.GetStorePath()` + `os.MkdirAll` block (lines 77–80):

```go
cat := b.catalog
if cat == nil {
	storePath, storePathErr := paths.Store()
	if storePathErr != nil {
		return nil, fmt.Errorf("quiver builder: %w", storePathErr)
	}
	store, storeErr := quiverstore.NewQuiverCatalog(filepath.Join(storePath, "quivers.db"))
	if storeErr != nil {
		return nil, storeErr
	}
	cat, err = catalog.New(axQuiver, store, v.Vault, v.Manifold)
	if err != nil {
		return nil, err
	}
}
```

Add `"github.com/rabbytesoftware/quiver/internal/core/paths"` to imports. Remove `"github.com/rabbytesoftware/quiver/internal/core/metadata"` and `"os"` if no longer referenced elsewhere in `builder.go`.

- [ ] **Step 5: Build to verify no compilation errors**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 6: Run full test suite**

```bash
go test ./... -count=1
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/container.go internal/engine/container.go internal/app/arrow/builder.go internal/app/quiver/builder.go
git commit -m "refactor: replace metadata+MkdirAll pairs with paths module"
```
