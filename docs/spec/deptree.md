# Quiver — DepTree

## Overview

DepTree is the infrastructure module responsible for **dependency resolution**. Given a root Arrow namespace and a way to look up each Arrow's direct dependencies, it builds the full dependency graph, detects cycles, and returns a valid installation order.

DepTree is pure graph logic. It does not fetch manifests, touch the filesystem, or know about Asynx. The app layer provides a resolver callback that handles manifest lookup — DepTree calls it and receives `[]Namespace` back.

### Call Site

DepTree is called exclusively during the **install use case** — the first phase of the async flow triggered by `POST /v1/arrow/{namespace}/_install`. It does **not** run during `arrow.Add` (adding an arrow to the catalog). This avoids blocking the synchronous add endpoint with potentially slow transitive manifest fetches.

The install flow:

```
POST /v1/arrow/{namespace}/_install → 202 Accepted
  1. runtime.Begin{_install} → state: installing
  2. DepTree.Resolve(root, resolver) → ordered []Namespace or error
     - If error (cycle, fetch failure) → fail the install, runtime.End{_install}
  3. For each dependency in topological order (excluding already-installed):
     a. resolveManifest (Vault cache → Manifold on miss)
     b. arrow.Add (if not already in catalog)
     c. runtime.Begin{_install} for the dependency
     d. Wizard executes install steps
     e. runtime.End{_install} for the dependency
  4. Wizard executes install steps for root arrow
  5. runtime.End{_install} for root arrow
  6. Update Vault entry for root with indirect_dependencies (see vault.md §4.5)
```

The resolver callback provided by the app layer checks Vault first (cache hit = fast), then falls back to Manifold (git fetch). This means dependencies that have been resolved before are essentially free; only truly new dependencies incur network latency.

---

## 1. Module Name

`deptree` — the dependency resolution module.

The package lives at `internal/infrastructure/deptree`.

---

## 2. Interface Contract

The app layer depends on a single interface:

```go
// DepTreePort is the interface the app layer depends on.
// It is defined in the app layer — deptree implements it.
type DepTreePort interface {
    // Resolve walks the dependency graph starting from root and returns
    // a topologically ordered list of namespaces. Dependencies appear
    // before their dependents; root is last.
    //
    // The resolver callback is called once per unique namespace to retrieve
    // its direct dependencies. The app layer constructs this callback,
    // typically wrapping Manifold + Vault lookups.
    //
    // Returns ErrCyclicDependency (wrapped in CycleError) if the graph
    // contains a cycle. Returns any error the resolver callback returns.
    Resolve(ctx context.Context, root Namespace, resolver ResolverFunc) ([]Namespace, error)
}

// ResolverFunc is the callback the app layer provides.
// Given a namespace, it returns that Arrow's direct dependencies.
// The app layer is responsible for fetching the manifest (via Vault or Manifold)
// and extracting the dependency list.
type ResolverFunc func(ctx context.Context, ns Namespace) ([]Namespace, error)
```

This is the **only** interface the app layer imports. No graph types, no internal state — just namespace in, ordered list out.

---

## 3. Algorithm

DepTree uses a **depth-first search (DFS) topological sort** with three-color marking for cycle detection.

### 3.1 Node States

Each namespace is tracked in one of three states during traversal:

| State | Meaning |
|-------|---------|
| **Unvisited** (white) | Not yet encountered |
| **In-progress** (gray) | Currently on the DFS stack — its subtree is being explored |
| **Done** (black) | Fully processed — all descendants resolved |

### 3.2 Traversal

```
resolve(root):
    order = []
    visited = {}    // black set
    inStack = {}    // gray set

    dfs(root):
        if root in visited:
            return              // already processed
        if root in inStack:
            return CycleError   // cycle detected

        inStack.add(root)
        deps = resolver(root)

        for dep in deps:
            dfs(dep)

        inStack.remove(root)
        visited.add(root)
        order.append(root)

    dfs(root)
    return order
```

The result is a **reverse post-order**: dependencies appear before their dependents. The root namespace is always the last element.

### 3.3 Cycle Detection

When DFS visits a node that is already in the gray (in-progress) set, a cycle exists. The cycle path is reconstructed from the DFS stack:

```
A → B → C → A    (cycle detected when revisiting A)
CycleError.Path = [A, B, C, A]
```

The full cycle path is included in the error for diagnostics.

### 3.4 Why DFS Over Kahn's Algorithm

Both DFS-based topological sort and Kahn's algorithm (BFS with in-degree tracking) produce valid topological orders. DFS is chosen because:

- **Cycle path extraction is free** — the DFS stack directly encodes the cycle when one is detected. Kahn's algorithm can detect that a cycle exists (not all nodes are consumed) but requires additional work to identify which nodes form the cycle.
- **Simpler implementation** — no need to precompute in-degrees or maintain a queue.
- **Lazy resolution** — nodes are only resolved (resolver callback called) when they are first visited. This avoids resolving manifests that are unreachable from the root.

---

## 4. Error Types

All errors are defined in the deptree package.

```go
var (
    // ErrCyclicDependency indicates the dependency graph contains a cycle.
    ErrCyclicDependency = errors.New("deptree: cyclic dependency detected")
)

// CycleError wraps ErrCyclicDependency with the full cycle path.
// The Path field contains the namespaces forming the cycle, with the
// repeated namespace appearing as both the first and last element.
type CycleError struct {
    Path []Namespace // e.g. [A, B, C, A]
}

func (e *CycleError) Error() string {
    // "deptree: cyclic dependency detected: github.com/a -> github.com/b -> github.com/c -> github.com/a"
    parts := make([]string, len(e.Path))
    for i, ns := range e.Path {
        parts[i] = ns.String()
    }
    return fmt.Sprintf("deptree: cyclic dependency detected: %s", strings.Join(parts, " -> "))
}

func (e *CycleError) Unwrap() error {
    return ErrCyclicDependency
}
```

Resolver callback errors propagate as-is — DepTree does not wrap them. The app layer can identify which namespace failed from the error context the resolver itself provides.

---

## 5. Edge Cases

### 5.1 Diamond Dependencies

```
A depends on B and C
B depends on D
C depends on D
```

D is visited once (via B's subtree). When C's DFS reaches D, it is already in the visited (black) set and is skipped. Output: `[D, B, C, A]` or `[D, C, B, A]` — both are valid topological orders.

### 5.2 Self-Dependency

```
A depends on A
```

A is marked gray, then resolver returns `[A]`. DFS visits A again — A is gray → cycle detected. `CycleError.Path = [A, A]`.

### 5.3 No Dependencies

```
A has no dependencies
```

Resolver returns `[]Namespace{}`. DFS completes immediately. Output: `[A]`.

### 5.4 Resolver Failure

If the resolver callback returns an error for any namespace, `Resolve` returns that error immediately. No partial results are returned. The app layer handles retries or fallback at its level.

### 5.5 Context Cancellation

The DFS checks `ctx.Err()` before each resolver call. If the context is cancelled, the traversal stops and returns the context error. This prevents long-running resolution from blocking when the caller has moved on.

---

## 6. Constraints

- **No Asynx knowledge** — DepTree is pure infrastructure. It does not emit events or commands.
- **No manifest knowledge** — DepTree does not know what an `ArrowManifest` is. It receives `[]Namespace` from the resolver callback and works only with namespaces.
- **No I/O** — DepTree performs no network calls, no disk reads. All external data comes through the resolver callback.
- **App layer is the only caller** — no other layer imports `DepTreePort`.
- **Deterministic output** — for a given graph, the output order is deterministic (DFS visits dependencies in the order returned by the resolver callback).
- **No caching** — DepTree does not cache resolver results across calls. Each `Resolve` call starts fresh. The app layer can cache manifests in the resolver callback closure if needed.

---

## 7. File Layout

```
internal/infrastructure/deptree/
    deptree.go       — DepTree struct, New(), Resolve()
    errors.go        — ErrCyclicDependency, CycleError
    deptree_test.go  — unit tests
```

---

## 8. Interaction with Vault

After DepTree resolves the full dependency graph, the app layer computes the **indirect dependencies** — all transitive dependencies that are not direct dependencies of the root arrow. This list is persisted on the Vault entry as `indirect_dependencies` (see `vault.md` §4.5).

DepTree itself does not write to Vault. It returns `[]Namespace` and the app layer decides what to persist.

---

## 9. Open Questions

| # | Question | Default if unresolved |
|---|----------|-----------------------|
| 1 | Should `Resolve` support a max-depth limit to prevent runaway resolution? | No limit in v0 — revisit if real-world graphs prove problematically deep. |
