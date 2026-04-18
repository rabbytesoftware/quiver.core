# Graph Layer Spec

This document specifies the `graph` package — Quiver's dependency graph component. It is
the authoritative source for how arrow dependency relationships are tracked, queried, and
maintained across versions.

Cross-references: [vault.md](./vault.md) · [deptree.md](./deptree.md) ·
[arrow/v0/versioning.md](./arrow/v0/versioning.md)

---

## 1. Purpose

The graph layer owns the dependency graph for the entire Arrow ecosystem. It answers two
classes of questions:

- **Forward:** what does Arrow A@v1.0.0 depend on, and under what constraints?
- **Reverse:** who depends on steamcmd@v2.0.1?

These questions arise during install, uninstall, update, and orphan detection. No other
layer maintains or queries dependency relationships directly — all dependency concerns are
delegated to `graph`.

---

## 2. Domain types

### 2.1 `DependencyEdge`

Represents one declared dependency from a specific arrow version toward another:

```go
type DependencyEdge struct {
    Namespace  domain.Namespace // concrete resolved version: steamcmd@v2.0.1
    Constraint string           // original declared constraint: "2.*", "", "v2.0.1"
    Type       DepType          // ToolDep | ServiceDep
}
```

`Namespace` always carries a concrete resolved ref — never a glob. The original glob is
preserved in `Constraint` so the update flow can re-evaluate eligibility later.

`Type` reflects whether the dep was declared under `tools:` (install-time) or `services:`
(runtime). This distinction drives behavior in the execution layer.

### 2.2 `DepType`

```go
type DepType string

const (
    ToolDep    DepType = "tool"
    ServiceDep DepType = "service"
)
```

---

## 3. `Graph` interface

```go
type Graph interface {
    // SaveEdges records all outgoing dependency edges for a specific arrow version.
    // Replaces any previously stored edges for that (namespace, version) pair.
    SaveEdges(
        ctx context.Context,
        fromNs domain.Namespace,
        fromVersion string,
        edges []DependencyEdge,
    ) error

    // DeleteEdges removes all outgoing dependency edges for a specific arrow version.
    // Called when an arrow version is removed from the catalog.
    DeleteEdges(
        ctx context.Context,
        fromNs domain.Namespace,
        fromVersion string,
    ) error

    // Dependents returns all edges pointing TO the given (namespace, version).
    // Each edge identifies a dependent and carries the constraint it declared.
    Dependents(
        ctx context.Context,
        toNs domain.Namespace,
        toVersion string,
    ) ([]DependencyEdge, error)

    // HasDependents reports whether any installed arrow other than excludeNs
    // depends on any version of ns.
    // Used for user-facing removal warnings (bare namespace level).
    HasDependents(
        ctx context.Context,
        ns domain.Namespace,
        excludeNs domain.Namespace,
    ) (bool, error)

    // CanUpgrade returns the installed (namespace, version) pairs whose declared
    // constraint for toNs permits upgrading from fromVersion to toVersion.
    // Used by the update flow to determine cascade eligibility.
    CanUpgrade(
        ctx context.Context,
        toNs domain.Namespace,
        fromVersion string,
        toVersion string,
    ) ([]DependencyEdge, error)
}
```

---

## 4. Storage — `dep_edges` table

The graph is persisted in a dedicated SQLite table within the same database as the
Arrow catalog store. It is maintained as a projection of Arrow aggregate events.

### 4.1 GORM model

```go
type depEdgeRow struct {
    FromNamespace string `gorm:"primaryKey;column:from_namespace"`
    FromVersion   string `gorm:"primaryKey;column:from_version"`
    ToNamespace   string `gorm:"primaryKey;column:to_namespace"`
    ToVersion     string `gorm:"not null;column:to_version"`
    Constraint    string `gorm:"not null;column:constraint"`
    DepType       string `gorm:"not null;column:dep_type"`
}

func (depEdgeRow) TableName() string { return "dep_edges" }
```

### 4.2 Indexes

| Index | Columns | Purpose |
|-------|---------|---------|
| PRIMARY KEY | `from_namespace, from_version, to_namespace` | uniqueness |
| `idx_dep_edges_to` | `to_namespace, to_version` | reverse lookup: who depends on X@v? |
| `idx_dep_edges_from` | `from_namespace, from_version` | forward lookup: what does X@v depend on? |

### 4.3 Query mapping

| `Graph` method | SQL pattern |
|---|---|
| `SaveEdges` | `DELETE WHERE from_namespace=? AND from_version=?` then batch `INSERT` |
| `DeleteEdges` | `DELETE WHERE from_namespace=? AND from_version=?` |
| `Dependents` | `SELECT * WHERE to_namespace=? AND to_version=?` |
| `HasDependents` | `SELECT 1 WHERE to_namespace=? AND from_namespace != ? LIMIT 1` |
| `CanUpgrade` | `SELECT * WHERE to_namespace=? AND to_version=? AND constraint` (glob-filtered in Go) |

`CanUpgrade` fetches all edges pointing to `(toNs, fromVersion)` and filters in Go using
glob matching against `toVersion`. SQL does not evaluate glob semantics.

---

## 5. Projection subscriptions

The graph registers subscriptions against the Arrow aggregate on construction. It never
receives commands — it only reacts to events.

| Event | Trigger | Graph action |
|-------|---------|--------------|
| `arrow.added` | Arrow registered in catalog | `SaveEdges` for each version's compiled targets |
| `arrow.updated` | Manifest refreshed | `SaveEdges` replacing previous edges for that version |
| `arrow.removed` | Arrow removed from catalog | `DeleteEdges` for all versions of that namespace |

Edges are populated at `arrow.added` time — the compiled targets already carry concrete
resolved deps (glob constraints resolved by the manifold resolver before compilation).
No separate post-install event is needed for the initial edge population.

---

## 6. `DependencyEdge` in the domain `Target`

`Target.Tools` and `Target.Services` change from `[]domain.Namespace` to
`[]DependencyEdge`. This carries both the concrete resolved namespace and the original
constraint into the compiled target, making it available to the graph projection and the
execution layer without requiring a separate lookup.

```go
type Target struct {
    Requirements Requirement
    Tools        []DependencyEdge  // was []Namespace
    Services     []DependencyEdge  // was []Namespace
    Exports      map[string]string
    Lifecycle    TargetLifecycle
    Methods      map[string]Method
}
```

The execution layer accesses `edge.Namespace` for `INSTALL_PATH` and export resolution.
The graph projection reads `edge.Constraint` and `edge.Type` when writing dep_edges.

---

## 7. Integration points

### 7.1 Catalog

`catalogService` holds a `graph.Graph` reference. On `catalog.Add` and `catalog.Update`,
after the `AddArrow` or `UpdateArrowManifest` command is sent, the catalog calls
`graph.SaveEdges` for each version in the compiled manifest.

`catalog.HasDependents` delegates entirely to `graph.HasDependents` — no in-catalog
scanning.

### 7.2 Execution layer

The installer calls `graph.HasDependents` before uninstalling to guard against removing
an arrow that other installed arrows depend on.

The runner accesses `target.Services` (now `[]DependencyEdge`) to determine which service
arrows must be running before `_execute` begins.

### 7.3 Update flow (Spec 2)

`graph.CanUpgrade` is called by the update use case to determine which dependent arrows
can have their dep edge upgraded from `fromVersion` to `toVersion` without violating their
declared constraint.

`graph.Dependents` is called to find which arrows need their services restarted after a
service dep updates.

---

## 8. Invariants

- Every edge in `dep_edges` references a concrete resolved version — never a glob — in
  `to_version` and `from_version`.
- `Constraint` may be a glob (`"2.*"`), an exact ref (`"v2.0.1"`), or empty (`""` for
  latest). It is the original value declared in the manifest before resolution.
- `SaveEdges` is idempotent — calling it twice for the same `(fromNs, fromVersion)` pair
  produces the same state as calling it once.
- The graph never drives side effects. It records and queries. The catalog and execution
  layers act on what the graph reports.
