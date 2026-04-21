# Quiver — DepTree

## Overview

DepTree is the infrastructure module responsible for **dependency resolution**. Given a root
Arrow ref and a way to look up each Arrow's direct dependencies, it builds the full dependency
graph, detects cycles, and returns a valid installation order.

DepTree is pure graph logic. It does not fetch manifests, touch the filesystem, or know about
Asynx. The app layer provides a resolver callback that handles manifest lookup — DepTree calls
it and receives `[]ArrowRef` back.

**Node identity is `ArrowRef` — a `(namespace, version)` pair.** `github.com/valve/steamcmd`
(latest) and `github.com/valve/steamcmd@v1.2.3` are different nodes; both may appear in the
same graph and both will be installed independently. Version mismatches between arrows are
never a conflict — see [versioning.md](arrow/v0/versioning.md).

### Call Site

DepTree is called during the **install use case** and the **uninstall use case** (for orphan ordering) — the first phase of the async flow triggered by `POST /v1/arrow/{namespace}/_install`. It does **not** run during `arrow.Add` (adding an arrow to the catalog). This avoids blocking the synchronous add endpoint with potentially slow transitive manifest fetches.

The install flow uses a synthetic **Step 0** of type `dependencies` to model DepTree resolution as a regular step in the execution. This gives unified progress tracking, natural error capture in `StepProgress.Error`, and consistent WebSocket events.

```
POST /v1/arrow/{namespace}/_install → 202 Accepted
  1. App layer prepends a dependencies step (type: "dependencies", index: 0)
     to the manifest's install steps (re-indexed from 1)
  2. runtime.Begin{_install, steps: [depStep, ...installSteps]} → state: installing
  3. runtime.Advance{0, running} — dependency resolution started
  4. DepTree.Resolve(root, resolver) → ordered []Namespace or error
     - If error → runtime.Advance{0, failed, error} → runtime.End{_install, failed} → state: absent
  5. runtime.Advance{0, completed} — dependencies resolved
  6. For each dependency in topological order (excluding already-installed):
     a. resolveManifest (Vault cache → Manifold on miss)
     b. arrow.Add (if not already in catalog)
     c. runtime.Begin{_install} for the dependency (its own step list with its own Step 0)
     d. Wizard executes install steps
     e. runtime.End{_install} for the dependency
     - If any dependency install fails → runtime.End{_install, failed} for root → state: absent
  7. Wizard executes install steps for root (starting from index 1 — Wizard skips Step 0)
  8. runtime.End{_install} for root → state: ready
  9. Update Vault entry for root with indirect_dependencies (see vault.md §4.5)
```

The resolver callback provided by the app layer checks Vault first (cache hit = fast), then falls back to Manifold (git fetch). This means dependencies that have been resolved before are essentially free; only truly new dependencies incur network latency.

**Partial failure with rollback:** If dependency A installs successfully but dependency B fails, the use case layer **rolls back** by uninstalling already-installed dependencies in **reverse order** (B is at `absent` — skip, A is at `ready` — uninstall). Rollback is best-effort: if a dependency's uninstall fails during rollback, the failure is logged and rollback continues with the remaining dependencies. After rollback completes (or partially completes), the root arrow transitions to `absent` (via `runtime.End{_install, failed}`). The root's `LastReturn` records which step failed: if Step 0 (`dependencies`) failed, the issue was resolution (cycle, fetch). If Step 0 succeeded but a dependency's install failed, the error is propagated to the root via `runtime.End`.

### Uninstall Flow

Before `_uninstall` begins, the use case layer performs a **reverse dependency check**: it scans all Vault entries to determine if any other installed arrow references the target in its `dependencies` or `indirect_dependencies`. If any other arrow depends on it, the uninstall is rejected with a 422 error. This prevents breaking other arrows' dependency chains.

When `_uninstall` completes successfully for an Arrow, the use case layer cleans up orphaned dependencies. DepTree itself is not called during uninstall — the use case layer performs the orphan check using Vault data.

#### Orphan Detection

For each namespace in the uninstalled arrow's `dependencies` + `indirect_dependencies` (sourced from the Vault entry):

1. Query all installed arrows' Vault entries
2. For each dep, check: does any OTHER installed arrow (not the one being uninstalled) list this dep in its `dependencies` or `indirect_dependencies`?
3. If no other arrow references it → the dep is orphaned

#### Uninstall Sequence

Orphaned dependencies are uninstalled in **reverse topological order** (leaves first — the opposite of install order). This ensures a dependency is only uninstalled after everything that depended on it has been removed.

```
POST /v1/arrow/{namespace}/_uninstall → 202 Accepted
  1. runtime.Begin{_uninstall} → state: uninstalling
  2. Wizard executes uninstall steps for root
  3. runtime.End{_uninstall, success} → state: removed
  4. Compute orphaned deps:
     For each dep in (root.vault.dependencies + root.vault.indirect_dependencies):
       if no other installed arrow references dep → orphaned
  5. Sort orphans in reverse topological order
  6. For each orphaned dep:
     a. runtime.Begin{_uninstall} → state: uninstalling
     b. Wizard executes uninstall steps
     c. runtime.End{_uninstall} → state: removed
  7. Clean up Vault entries for removed arrows
```

Every step flows through Asynx — subscribers see each dependency's uninstall as regular `runtime.Begin`, `runtime.Advance`, and `runtime.End` events.

#### Partial Failure

If an orphaned dependency's uninstall fails, it transitions to `ready` (rollback — per the existing `runtime.End` state machine). The remaining orphans are still attempted. The root arrow's uninstall is already complete and unaffected.

---

## 1. Module Name

`deptree` — the dependency resolution module.

The package lives at `internal/infrastructure/deptree`.

---

## 2. Interface Contract

The app layer depends on a single interface:

```go
// ArrowRef identifies a specific installation of an Arrow — a (namespace, version) pair.
// Version is the @ref suffix from the manifest declaration (e.g. "v1.2.3", "main").
// An empty Version means "latest" — HEAD of the default branch.
type ArrowRef struct {
    Namespace Namespace
    Version   string // empty = latest
}

// DepTree is the interface the app layer depends on.
// Defined in the deptree package.
type DepTree interface {
    // Resolve walks the dependency graph starting from root and returns
    // a topologically ordered list of ArrowRefs. Dependencies appear
    // before their dependents; root is last.
    //
    // The resolver callback is called once per unique ArrowRef to retrieve
    // its direct dependencies. The app layer constructs this callback,
    // typically wrapping Manifold + Vault lookups.
    //
    // Returns ErrCyclicDependency (wrapped in CycleError) if the graph
    // contains a cycle. Returns any error the resolver callback returns.
    Resolve(ctx context.Context, root ArrowRef, resolver ResolverFunc) ([]ArrowRef, error)
}

// ResolverFunc is the callback the app layer provides.
// Given an ArrowRef, it returns that Arrow's direct dependencies as ArrowRefs.
// The app layer is responsible for fetching the manifest (via Vault or Manifold)
// and extracting the dependency list with their declared versions.
type ResolverFunc func(ctx context.Context, ref ArrowRef) ([]ArrowRef, error)
```

This is the **only** interface the app layer imports. No graph types, no internal state — just
`ArrowRef` in, ordered list out. Two refs with the same namespace but different versions are
distinct nodes — no special handling required.

---

## 3. Algorithm

DepTree uses a **depth-first search (DFS) topological sort** with three-color marking for cycle detection.

### 3.1 Node States

Each ArrowRef is tracked in one of three states during traversal:

| State | Meaning |
|-------|---------|
| **Unvisited** (white) | Not yet encountered |
| **In-progress** (gray) | Currently on the DFS stack — its subtree is being explored |
| **Done** (black) | Fully processed — all descendants resolved |

### 3.2 Traversal

```
resolve(root ArrowRef):
    order = []
    visited = {}    // black set — keyed by (namespace, version)
    inStack = {}    // gray set — keyed by (namespace, version)

    dfs(ref ArrowRef):
        if ref in visited:
            return              // already processed
        if ref in inStack:
            return CycleError   // cycle detected

        inStack.add(ref)
        deps = resolver(ref)    // returns []ArrowRef with versions from manifest

        for dep in deps:
            dfs(dep)

        inStack.remove(ref)
        visited.add(ref)
        order.append(ref)

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
// The Path field contains the ArrowRefs forming the cycle, with the
// repeated ref appearing as both the first and last element.
type CycleError struct {
    Path []ArrowRef // e.g. [A, B, C, A]
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
B depends on D@v1.0.0
C depends on D@v1.0.0
```

`D@v1.0.0` is visited once (via B's subtree). When C's DFS reaches `D@v1.0.0`, it is already
in the visited (black) set and is skipped. Output: `[D@v1.0.0, B, C, A]` or
`[D@v1.0.0, C, B, A]` — both are valid topological orders. One installation of `D@v1.0.0`
is shared between B and C.

### 5.6 Version Mismatch (not an error)

```
A depends on B and C
B depends on D@v1.0.0
C depends on D@v1.2.0
```

`D@v1.0.0` and `D@v1.2.0` are different `ArrowRef` values — different nodes. Both are visited,
both are installed into their respective version subdirectories. Output:
`[D@v1.0.0, D@v1.2.0, B, C, A]` (order of the two D installs depends on traversal order).
No conflict error is raised. See [versioning.md](arrow/v0/versioning.md).

### 5.2 Self-Dependency

```
A depends on A
```

A is marked gray, then resolver returns `[A]`. DFS visits A again — A is gray → cycle detected. `CycleError.Path = [A, A]`.

### 5.3 No Dependencies

```
A has no dependencies
```

Resolver returns `[]ArrowRef{}`. DFS completes immediately. Output: `[A]`.

### 5.4 Resolver Failure

If the resolver callback returns an error for any namespace, `Resolve` returns that error immediately. No partial results are returned. The app layer handles retries or fallback at its level.

### 5.5 Context Cancellation

The DFS checks `ctx.Err()` before each resolver call. If the context is cancelled, the traversal stops and returns the context error. This prevents long-running resolution from blocking when the caller has moved on.

---

## 6. Constraints

- **No Asynx knowledge** — DepTree is pure infrastructure. It does not emit events or commands.
- **No manifest knowledge** — DepTree does not know what an `ArrowManifest` is. It receives `[]ArrowRef` from the resolver callback and works only with refs.
- **No I/O** — DepTree performs no network calls, no disk reads. All external data comes through the resolver callback.
- **App layer is the only caller** — no other layer imports `DepTree`.
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

After DepTree resolves the full dependency graph, the app layer computes the **indirect
dependencies** — all transitive dependencies that are not direct dependencies of the root
arrow. This list is persisted on the Vault entry as `indirect_dependencies`
(see `vault.md` §4.5).

DepTree itself does not write to Vault. It returns `[]ArrowRef` and the app layer decides what
to persist. Because the result carries versions, `indirect_dependencies` on the Vault entry
is `[]ArrowRef` (namespace + version) rather than bare namespaces — the full versioned graph
is preserved for accurate orphan detection at uninstall time.

---

## 9. Open Questions

| # | Question | Default if unresolved |
|---|----------|-----------------------|
| 1 | Should `Resolve` support a max-depth limit to prevent runaway resolution? | No limit in v0 — revisit if real-world graphs prove problematically deep. |
| 2 | Should cycles be detected across versions? (`A@v1` depends on `A@v2` depends on `A@v1`) | Treat as a cycle — same namespace appearing twice in the in-progress set regardless of version is an error. |
