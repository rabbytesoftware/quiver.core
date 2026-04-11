# Arrow Seed & Validate Design

**Date:** 2026-04-11
**Branch:** enhancement/v0-first-boot-fixes
**Status:** Approved

## Summary

Two new `SEED` endpoints that allow developers to work with arrow manifests locally — without pulling from a remote git repository:

| Verb | Path | Purpose |
|------|------|---------|
| `SEED` | `/arrow/:ns/validate` | Translate + Assemble a raw YAML manifest, return structured errors |
| `SEED` | `/arrow/:ns` | Add an arrow to the catalog from a local YAML manifest |

After seeding, the existing `POST /arrow/:ns/install` installs the arrow unchanged.

---

## Context

The Manifold engine pipeline is: **Resolver → Translator → Assembler**.

- **Resolver** fetches raw YAML bytes from a remote git/HTTP source.
- **Translator** parses raw bytes into `*domain.ArrowManifest`, selecting the correct version mapper (currently v0).
- **Assembler** validates business rules on the parsed manifest.

Both new endpoints skip the Resolver and provide raw bytes directly. The new `Manifold.ParseArrow` method encapsulates the Translate → Assemble path.

---

## HTTP Design

### Custom verb: `SEED`

`SEED` is registered as a custom HTTP method via Gin's `router.Handle("SEED", path, handler)`. Since Quiver's API is a local daemon (no proxies, CDNs, or strict CORS), custom verbs are safe to use. `SEED` is semantically distinct from `POST` (remote fetch) — it means "here is the manifest, use it directly."

### Why no conflict with `POST /arrow/:ns/:method`

Gin routes by HTTP method + path in separate radix trees. `SEED /arrow/:ns/validate` and `POST /arrow/:ns/:method` never collide.

### Why `:ns` on the validate endpoint

Validation is namespaced for semantic consistency: you're validating what you intend to seed at a given namespace. It mirrors the `SEED /arrow/:ns` path cleanly and leaves room for future namespace-aware checks (e.g. duplicate detection).

---

## Component Changes

### 1. Assembler — structured errors

**File:** `internal/engine/manifold/assembler/assembler.go` (and `rules.go`)

Add `AssemblerError` and `AssemblerErrors` types:

```go
type AssemblerError struct {
    Field   string // e.g. "lifecycle.install", "variables[1].min"
    Rule    string // e.g. "missing_pair", "invalid_range"
    Message string // human-readable description
}

type AssemblerErrors []AssemblerError

func (e AssemblerErrors) Error() string { ... }
```

`ValidateArrow` returns `AssemblerErrors` (nil on success). Each rule in `rules.go` maps its failure to one `AssemblerError` entry. Callers checking only `err != nil` continue to work unchanged.

### 2. Manifold — `ParseArrow`

**File:** `internal/engine/manifold/manifold.go` + implementation + mock

New method on the `Manifold` interface:

```go
ParseArrow(data []byte) (*domain.ArrowManifest, error)
```

Implementation:
1. `Translator.Arrow(data)` — translate raw bytes to domain model
2. `Assembler.ValidateArrow(manifest)` — validate business rules
3. Return manifest on success, `AssemblerErrors` on failure

No Resolver involved. The mock gets this method too.

### 3. Catalog — `AddWithManifest`

**File:** `internal/app/arrow/internal/catalog/catalog.go`

New method on the `Catalog` interface:

```go
AddWithManifest(ctx context.Context, ns domain.Namespace, manifest *domain.ArrowManifest) error
```

Same internals as `Add` but skips `resolveManifest()` — calls `Vault.PutArrow` directly and emits the Arrow event. The manifest is already parsed and validated by the time it arrives here.

### 4. ArrowService — `Seed` and `ValidateManifest`

**File:** `internal/app/arrow/arrow.go`

New methods on the `ArrowService` interface and implementation:

```go
Seed(ctx context.Context, ns domain.Namespace, data []byte) error
ValidateManifest(ctx context.Context, ns domain.Namespace, data []byte) (*ValidationResult, error)
```

**`Seed` flow:**
1. `Manifold.ParseArrow(data)` → `*domain.ArrowManifest` (or error)
2. `Catalog.AddWithManifest(ctx, ns, manifest)`

**`ValidateManifest` flow:**
1. `Manifold.ParseArrow(data)` → on success: `&ValidationResult{Valid: true}`
2. On `AssemblerErrors`: `&ValidationResult{Valid: false, Errors: [...]}`

`ValidationResult` and `ValidationError` live in the app layer:

```go
type ValidationResult struct {
    Valid  bool
    Errors []ValidationError
}

type ValidationError struct {
    Field   string
    Rule    string
    Message string
}
```

### 5. API — DTOs

**File:** `internal/api/v0/dto/` (new file)

```go
type ValidationResultDTO struct {
    Valid  bool                 `json:"valid"`
    Errors []ValidationErrorDTO `json:"errors,omitempty"`
}

type ValidationErrorDTO struct {
    Field   string `json:"field"`
    Rule    string `json:"rule"`
    Message string `json:"message"`
}
```

Mapped from `arrow.ValidationResult` via a `ValidationResultDTOFrom` constructor.

### 6. API — Handlers

**File:** `internal/api/v0/endpoints/arrows/handlers/handlers.go`

Two new handler methods on `Handlers`:

- **`Validate`**: reads raw request body → calls `svc.ValidateManifest(ctx, ns, body)` → writes `ValidationResultDTO` with `200 OK` (always 200; `valid: false` is a valid response, not an HTTP error)
- **`Seed`**: reads raw request body → calls `svc.Seed(ctx, ns, body)` → writes `201 Created` on success

### 7. API — Routes

**File:** `internal/api/v0/endpoints/arrows/routes.go`

```go
rg.Handle("SEED", "/arrow/:ns", h.Seed)
rg.Handle("SEED", "/arrow/:ns/validate", h.Validate)
```

---

## Data Flow Diagram

```
SEED /arrow/:ns/validate
  └─ body ([]byte)
       └─ ArrowService.ValidateManifest(ctx, ns, []byte)
            └─ Manifold.ParseArrow([]byte)
                 ├─ Translator.Arrow([]byte)   → *domain.ArrowManifest
                 └─ Assembler.ValidateArrow()  → AssemblerErrors | nil
            └─ ValidationResult{Valid, Errors}
       └─ ValidationResultDTO → 200 OK

SEED /arrow/:ns
  └─ body ([]byte)
       └─ ArrowService.Seed(ctx, ns, []byte)
            └─ Manifold.ParseArrow([]byte)    → *domain.ArrowManifest
            └─ Catalog.AddWithManifest(ctx, ns, manifest)
                 └─ Vault.PutArrow(...)
                 └─ emit Arrow event
       └─ 201 Created

POST /arrow/:ns/install   (unchanged)
  └─ ArrowService.BeginExecution(ctx, ns, "install", vars)
```

---

## Testing

- **Assembler**: existing tests updated to assert `AssemblerErrors` fields (field, rule, message) rather than plain error strings.
- **Manifold**: unit test for `ParseArrow` with valid YAML, invalid YAML, and business rule violations.
- **Catalog**: unit test for `AddWithManifest` — verifies Vault.PutArrow is called with the provided manifest and no Manifold fetch occurs.
- **ArrowService**: unit tests for `Seed` and `ValidateManifest` using mock Manifold and Catalog.
- **Handlers**: handler tests for both `SEED` routes covering success, validation failure, and bad body.
