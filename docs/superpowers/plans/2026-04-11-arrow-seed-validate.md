# Arrow Seed & Validate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `SEED /arrow/:ns` (local manifest install) and `SEED /arrow/:ns/validate` (structured manifest validation) endpoints to the Quiver API.

**Architecture:** Five layers top-to-bottom: Assembler → Manifold → Catalog → ArrowService → API. Each task is independent from the one after it. Start from the deepest layer and work up so each layer can be tested in isolation before the next depends on it.

**Tech Stack:** Go, Gin, testify, asynx event sourcing, SQLite via modernc

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `internal/engine/manifold/assembler/errors.go` | Modify | Add `AssemblerError`, `AssemblerErrors` types |
| `internal/engine/manifold/assembler/rules.go` | Modify | Rules return `AssemblerErrors`, collect-all |
| `internal/engine/manifold/assembler/assembler.go` | Modify | `ValidateArrow` collects errors from all rules |
| `internal/engine/manifold/assembler/rules_test.go` | Modify | Update `errors.Is` checks (still works via Unwrap) |
| `internal/engine/manifold/assembler/assembler_test.go` | Modify | Add field/rule assertions on `AssemblerErrors` |
| `internal/engine/manifold/manifold.go` | Modify | Add `ParseArrow([]byte)` to interface + impl |
| `internal/engine/manifold/manifold_test.go` | Modify | Tests for `ParseArrow` |
| `internal/mocks/manifold.go` | Modify | Add `ParseArrow` to mock |
| `internal/app/arrow/internal/catalog/catalog.go` | Modify | Add `AddWithManifest` to interface + impl |
| `internal/app/arrow/internal/catalog/catalog_test.go` | Modify | Tests for `AddWithManifest` |
| `internal/app/errors/errors.go` | Modify | Add `ErrInvalidManifest` |
| `internal/api/libs/apierr/mapper.go` | Modify | Map `ErrInvalidManifest` → 422 |
| `internal/app/arrow/types.go` | Modify | Add `ValidationResult`, `ValidationError` |
| `internal/app/arrow/arrow.go` | Modify | Add `manifold` field, `Seed`, `ValidateManifest` |
| `internal/app/arrow/builder.go` | Modify | Wire `manifold: e.Manifold` into `arrowService` |
| `internal/app/arrow/arrow_test.go` | Modify | Extend `mockCatalog`, add service tests |
| `internal/api/v0/dto/arrow_validation.go` | Create | `ValidationResultDTO`, `ValidationErrorDTO` |
| `internal/api/v0/endpoints/arrows/handlers/handlers.go` | Modify | Add `Validate`, `Seed` handlers |
| `internal/api/v0/endpoints/arrows/handlers/handlers_test.go` | Modify | Handler tests for both SEED endpoints |
| `internal/api/v0/endpoints/arrows/routes.go` | Modify | Register `SEED /arrow/:ns` and `SEED /arrow/:ns/validate` |
| `internal/api/mocks/arrow_service.go` | Modify | Add `Seed`, `ValidateManifest` to mock |

---

## Task 1: Assembler — structured error types

**Files:**
- Modify: `internal/engine/manifold/assembler/errors.go`
- Modify: `internal/engine/manifold/assembler/rules.go`
- Modify: `internal/engine/manifold/assembler/assembler.go`
- Modify: `internal/engine/manifold/assembler/rules_test.go`
- Modify: `internal/engine/manifold/assembler/assembler_test.go`

- [ ] **Step 1: Add `AssemblerError` and `AssemblerErrors` to `errors.go`**

Replace the entire file contents:

```go
package assembler

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidManifest is the sentinel that all AssemblerErrors unwrap to.
var ErrInvalidManifest = errors.New("assembler: invalid manifest")

// ErrUnsupportedPlatform is returned when the requested OS is not listed in
// the Arrow's requirements.
var ErrUnsupportedPlatform = errors.New("assembler: unsupported platform")

// AssemblerError is a single validation failure with a structured location,
// rule name, and human-readable message.
type AssemblerError struct {
	Field   string // YAML path, e.g. "lifecycle.install", "variables[1].min"
	Rule    string // machine-readable rule ID, e.g. "missing_pair", "invalid_range"
	Message string // human-readable description
}

func (e AssemblerError) Error() string {
	return fmt.Sprintf("%s: %s [%s]: %s", ErrInvalidManifest, e.Field, e.Rule, e.Message)
}

// Unwrap lets errors.Is(err, ErrInvalidManifest) work on any AssemblerError.
func (e AssemblerError) Unwrap() error {
	return ErrInvalidManifest
}

// AssemblerErrors is the collection type returned by ValidateArrow.
// It implements error so it can be used wherever error is expected.
type AssemblerErrors []AssemblerError

func (e AssemblerErrors) Error() string {
	msgs := make([]string, len(e))
	for i, ae := range e {
		msgs[i] = ae.Error()
	}
	return strings.Join(msgs, "; ")
}

// Unwrap implements the multi-error interface so errors.Is walks each entry.
func (e AssemblerErrors) Unwrap() []error {
	errs := make([]error, len(e))
	for i, ae := range e {
		errs[i] = ae
	}
	return errs
}
```

- [ ] **Step 2: Update `rules.go` — all rule functions return `AssemblerErrors`**

Replace the entire file:

```go
package assembler

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

func validateLifecyclePairs(lc domain.Lifecycle) AssemblerErrors {
	var errs AssemblerErrors

	hasInstall := len(lc.Install) > 0
	hasUninstall := len(lc.Uninstall) > 0
	if hasInstall != hasUninstall {
		errs = append(errs, AssemblerError{
			Field:   "lifecycle.install",
			Rule:    "missing_pair",
			Message: "install and uninstall must both be defined or both be empty",
		})
	}

	hasExecute := len(lc.Execute) > 0
	hasStop := len(lc.Stop) > 0
	if hasExecute != hasStop {
		errs = append(errs, AssemblerError{
			Field:   "lifecycle.execute",
			Rule:    "missing_pair",
			Message: "execute and stop must both be defined or both be empty",
		})
	}

	return errs
}

func validateDependencies(deps []domain.Namespace) AssemblerErrors {
	var errs AssemblerErrors
	for i, d := range deps {
		if err := d.Validate(); err != nil {
			errs = append(errs, AssemblerError{
				Field:   fmt.Sprintf("dependencies[%d]", i),
				Rule:    "invalid_namespace",
				Message: fmt.Sprintf("invalid dependency %q: %v", d, err),
			})
		}
	}
	return errs
}

func validateVariables(vars []domain.Variable) AssemblerErrors {
	var errs AssemblerErrors
	names := make([]string, 0, len(vars))

	for i, v := range vars {
		if err := v.Validate(); err != nil {
			errs = append(errs, AssemblerError{
				Field:   fmt.Sprintf("variables[%d]", i),
				Rule:    "invalid_variable",
				Message: err.Error(),
			})
		}

		if v.Type.IsSelect() && len(v.Values) == 0 {
			errs = append(errs, AssemblerError{
				Field:   fmt.Sprintf("variables[%d].values", i),
				Rule:    "missing_values",
				Message: fmt.Sprintf("select variable %q must have at least one value", v.Name),
			})
		}

		names = append(names, v.Name)
	}

	if ae := checkDuplicates(names, "variables", "duplicate_name"); ae != nil {
		errs = append(errs, *ae)
	}

	return errs
}

func validateNetbridge(ports []netbridge.PortDef) AssemblerErrors {
	var errs AssemblerErrors
	names := make([]string, 0, len(ports))

	for i, p := range ports {
		if err := p.Validate(); err != nil {
			errs = append(errs, AssemblerError{
				Field:   fmt.Sprintf("netbridge[%d]", i),
				Rule:    "invalid_port",
				Message: err.Error(),
			})
		}
		names = append(names, p.Name)
	}

	if ae := checkDuplicates(names, "netbridge", "duplicate_name"); ae != nil {
		errs = append(errs, *ae)
	}

	return errs
}

func validateMethodStates(methods map[string]domain.Method) AssemblerErrors {
	validStates := map[string]struct{}{
		string(domain.ArrowStateReady):   {},
		string(domain.ArrowStateRunning): {},
	}

	var errs AssemblerErrors
	for name, m := range methods {
		for _, state := range m.AvailableIn {
			if _, ok := validStates[string(state)]; !ok {
				errs = append(errs, AssemblerError{
					Field:   fmt.Sprintf("methods[%s].available_in", name),
					Rule:    "invalid_state",
					Message: fmt.Sprintf("method %q has invalid state %q (must be ready or running)", name, state),
				})
			}
		}
	}
	return errs
}

func checkDuplicates(names []string, field, rule string) *AssemblerError {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			return &AssemblerError{
				Field:   field,
				Rule:    rule,
				Message: fmt.Sprintf("duplicate name %q", name),
			}
		}
		seen[name] = struct{}{}
	}
	return nil
}
```

- [ ] **Step 3: Update `assembler.go` — collect all rule errors**

Replace the entire file:

```go
package assembler

import "github.com/rabbytesoftware/quiver/internal/domain"

// ValidateArrow applies all business rules to a domain.ArrowManifest.
// It runs every rule and returns all violations as AssemblerErrors (nil on success).
func ValidateArrow(
	manifest *domain.ArrowManifest,
) error {
	var errs AssemblerErrors
	errs = append(errs, validateLifecyclePairs(manifest.Lifecycle)...)
	errs = append(errs, validateVariables(manifest.Variables)...)
	errs = append(errs, validateNetbridge(manifest.Netbridge)...)
	errs = append(errs, validateDependencies(manifest.Dependencies)...)
	errs = append(errs, validateMethodStates(manifest.Methods)...)
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ValidateQuiver applies all business rules to a domain.QuiverManifest.
func ValidateQuiver(
	manifest *domain.QuiverManifest,
) error {
	return nil
}
```

- [ ] **Step 4: Run assembler tests**

```bash
go test ./internal/engine/manifold/assembler/... -v -run .
```

Expected: all existing tests PASS (`errors.Is` still works via `AssemblerError.Unwrap() → ErrInvalidManifest`).

- [ ] **Step 5: Add structured-error assertions to `assembler_test.go`**

Add after the existing `TestValidateArrow_MissingUninstall` test:

```go
func TestValidateArrow_MissingUninstall_HasStructuredError(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
		},
	}
	err := assembler.ValidateArrow(manifest)
	require.Error(t, err)

	var asmErrs assembler.AssemblerErrors
	require.ErrorAs(t, err, &asmErrs)
	require.NotEmpty(t, asmErrs)
	assert.Equal(t, "lifecycle.install", asmErrs[0].Field)
	assert.Equal(t, "missing_pair", asmErrs[0].Rule)
}

func TestValidateArrow_CollectsMultipleErrors(t *testing.T) {
	// Both lifecycle pair AND variable errors should be collected.
	manifest := &domain.ArrowManifest{
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
			// missing uninstall
		},
		Variables: []domain.Variable{
			{Name: "VAR1"},
			{Name: "VAR1"}, // duplicate
		},
	}
	err := assembler.ValidateArrow(manifest)
	require.Error(t, err)

	var asmErrs assembler.AssemblerErrors
	require.ErrorAs(t, err, &asmErrs)
	assert.Greater(t, len(asmErrs), 1, "expected multiple errors to be collected")
}
```

Add the missing imports at the top of `assembler_test.go`:
```go
import (
    "testing"
    "time"

    "github.com/rabbytesoftware/quiver/internal/domain"
    "github.com/rabbytesoftware/quiver/internal/domain/netbridge"
    "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
    "github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
```

- [ ] **Step 6: Run assembler tests again**

```bash
go test ./internal/engine/manifold/assembler/... -v -run .
```

Expected: all tests PASS including the two new ones.

- [ ] **Step 7: Commit**

```bash
git add internal/engine/manifold/assembler/
git commit -m "feat(assembler): structured AssemblerErrors with field/rule/message"
```

---

## Task 2: Manifold — `ParseArrow`

**Files:**
- Modify: `internal/engine/manifold/manifold.go`
- Modify: `internal/engine/manifold/manifold_test.go`
- Modify: `internal/mocks/manifold.go`

- [ ] **Step 1: Add `ParseArrow` to the `Manifold` interface and implement it in `manifold.go`**

In the interface, add after `ResolveQuiver`:

```go
// ParseArrow translates and validates a raw YAML arrow manifest without
// fetching from a remote source. Returns AssemblerErrors if validation fails.
ParseArrow(data []byte) (*domain.ArrowManifest, error)
```

In the `manifold` struct, add the method:

```go
func (m *manifold) ParseArrow(data []byte) (*domain.ArrowManifest, error) {
	manifest, err := m.trs.Arrow(data)
	if err != nil {
		return nil, err
	}
	if err := assembler.ValidateArrow(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}
```

- [ ] **Step 2: Write failing tests for `ParseArrow` in `manifold_test.go`**

Add after the existing quiver tests:

```go
func TestParseArrow_TranslatorError(t *testing.T) {
	translateErr := errors.New("bad yaml")
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrowErr: translateErr},
	}
	_, err := m.ParseArrow([]byte("bad yaml"))
	if !errors.Is(err, translateErr) {
		t.Fatalf("expected translateErr, got %v", err)
	}
}

func TestParseArrow_AssemblerError_ReturnsStructuredErrors(t *testing.T) {
	invalidManifest := &domain.ArrowManifest{
		Name:    "test",
		Version: "1.0.0",
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "install.sh", 0, true),
			},
			// missing uninstall — assembler will catch this
		},
	}
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: invalidManifest},
	}
	_, err := m.ParseArrow([]byte("any"))
	if err == nil {
		t.Fatal("expected assembler error")
	}
	var asmErrs assembler.AssemblerErrors
	if !errors.As(err, &asmErrs) {
		t.Fatalf("expected AssemblerErrors, got %T: %v", err, err)
	}
}

func TestParseArrow_ValidManifest_ReturnsManifest(t *testing.T) {
	validManifest := &domain.ArrowManifest{
		Name:    "my-arrow",
		Version: "1.0.0",
	}
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: validManifest},
	}
	result, err := m.ParseArrow([]byte("any"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "my-arrow" {
		t.Errorf("Name = %q, want my-arrow", result.Name)
	}
}
```

Add import for assembler package at the top of `manifold_test.go`:
```go
"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
```

- [ ] **Step 3: Run manifold tests to verify they fail**

```bash
go test ./internal/engine/manifold/ -v -run TestParseArrow
```

Expected: FAIL — `ParseArrow` not defined yet (compile error).

- [ ] **Step 4: Run manifold tests after implementing**

```bash
go test ./internal/engine/manifold/... -v -run .
```

Expected: all PASS.

- [ ] **Step 5: Add `ParseArrow` to `internal/mocks/manifold.go`**

Add field to the struct and implement the method:

```go
type Manifold struct {
	ResolveArrowManifest *domain.ArrowManifest
	ResolveArrowErr      error
	ResolveQuiverManifest *domain.QuiverManifest
	ResolveQuiverErr     error
	ParseArrowManifest   *domain.ArrowManifest
	ParseArrowErr        error
}

func (m *Manifold) ParseArrow(data []byte) (*domain.ArrowManifest, error) {
	return m.ParseArrowManifest, m.ParseArrowErr
}
```

- [ ] **Step 6: Run all manifold-related tests**

```bash
go test ./internal/engine/manifold/... ./internal/mocks/... -v -run .
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/engine/manifold/manifold.go internal/engine/manifold/manifold_test.go internal/mocks/manifold.go
git commit -m "feat(manifold): add ParseArrow — translate+assemble without remote fetch"
```

---

## Task 3: Catalog — `AddWithManifest`

**Files:**
- Modify: `internal/app/arrow/internal/catalog/catalog.go`
- Modify: `internal/app/arrow/internal/catalog/catalog_test.go`

- [ ] **Step 1: Write a failing test for `AddWithManifest` in `catalog_test.go`**

Add after the existing `TestAdd_*` tests. First read `catalog_test.go` to understand the `testCatalog` helper, then add:

```go
func TestAddWithManifest_StoresManifestInVaultAndEmitsEvent(t *testing.T) {
	mv := &mocks.Vault{PutArrowPath: "/tmp/test"}
	mm := &mocks.Manifold{}
	cs, _ := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/user/repo")
	manifest := makeManifest("test-arrow")

	err := cs.AddWithManifest(context.Background(), ns, manifest)
	require.NoError(t, err)

	// Arrow should now be retrievable from catalog.
	cs.axArrow.WaitPublish()
	got, err := cs.axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "test-arrow", got.Manifest.Name)
}

func TestAddWithManifest_InvalidNamespace_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{}
	mm := &mocks.Manifold{}
	_, cat := testCatalog(t, mv, mm)

	err := cat.AddWithManifest(context.Background(), "bad", &domain.ArrowManifest{})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestAddWithManifest_VaultPutFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{PutArrowErr: errors.New("disk full")}
	mm := &mocks.Manifold{}
	_, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/user/repo")
	err := cat.AddWithManifest(context.Background(), ns, makeManifest("x"))
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/app/arrow/internal/catalog/... -v -run TestAddWithManifest
```

Expected: FAIL — compile error, `AddWithManifest` not defined.

- [ ] **Step 3: Add `AddWithManifest` to the `Catalog` interface in `catalog.go`**

In the `Catalog` interface, add:

```go
AddWithManifest(
    ctx context.Context,
    ns domain.Namespace,
    manifest *domain.ArrowManifest,
) error
```

- [ ] **Step 4: Implement `AddWithManifest` on `catalogService` in `catalog.go`**

Add after the `Add` method:

```go
func (c *catalogService) AddWithManifest(
	ctx context.Context,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add arrow with manifest: %w", apperrors.ErrInvalidNamespace)
	}

	if _, err := c.vault.PutArrow(ctx, ns, manifest, nil); err != nil {
		return fmt.Errorf("add arrow with manifest: %w", err)
	}

	if _, err := c.axArrow.Send(ctx, arrowcmds.AddArrow{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		return fmt.Errorf("add arrow with manifest: %w", err)
	}

	return nil
}
```

- [ ] **Step 5: Run catalog tests**

```bash
go test ./internal/app/arrow/internal/catalog/... -v -run .
```

Expected: all PASS. Note: the `mocks.Vault` must have a `PutArrow` method — it already does based on the existing test setup.

- [ ] **Step 6: Commit**

```bash
git add internal/app/arrow/internal/catalog/
git commit -m "feat(catalog): add AddWithManifest — store pre-parsed manifest without remote fetch"
```

---

## Task 4: App layer — errors, types, service methods, wiring

**Files:**
- Modify: `internal/app/errors/errors.go`
- Modify: `internal/api/libs/apierr/mapper.go`
- Modify: `internal/app/arrow/types.go`
- Modify: `internal/app/arrow/arrow.go`
- Modify: `internal/app/arrow/builder.go`
- Modify: `internal/app/arrow/arrow_test.go`
- Modify: `internal/api/mocks/arrow_service.go`

- [ ] **Step 1: Add `ErrInvalidManifest` to `internal/app/errors/errors.go`**

Add the new sentinel to the var block:

```go
ErrInvalidManifest = errors.New("invalid manifest")
```

- [ ] **Step 2: Map `ErrInvalidManifest` in `internal/api/libs/apierr/mapper.go`**

Add a new case before `default`:

```go
case errors.Is(err, apperrors.ErrInvalidManifest):
    return http.StatusUnprocessableEntity, "invalid manifest"
```

- [ ] **Step 3: Add `ValidationResult` and `ValidationError` to `internal/app/arrow/types.go`**

Append to the file:

```go
// ValidationResult is returned by ValidateManifest.
// Valid is true when the manifest passes all assembler rules.
// On failure, Errors contains one entry per violated rule.
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// ValidationError is a single structured validation failure.
type ValidationError struct {
	Field   string
	Rule    string
	Message string
}
```

- [ ] **Step 4: Write failing tests for `Seed` and `ValidateManifest` in `arrow_test.go`**

First, extend `mockCatalog` to implement the new `AddWithManifest` method:

```go
func (m *mockCatalog) AddWithManifest(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.ArrowManifest,
) error {
	return m.addErr
}
```

Then add a `mockManifold` struct for service-level tests:

```go
type mockManifold struct {
	parseManifest *domain.ArrowManifest
	parseErr      error
}

func (m *mockManifold) ResolveArrow(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) {
	return nil, errors.New("not used in these tests")
}

func (m *mockManifold) ResolveQuiver(_ context.Context, _ domain.Namespace) (*domain.QuiverManifest, error) {
	return nil, errors.New("not used in these tests")
}

func (m *mockManifold) ParseArrow(_ []byte) (*domain.ArrowManifest, error) {
	return m.parseManifest, m.parseErr
}
```

Update `newTestService` to accept and wire a manifold:

```go
func newTestService(t *testing.T, cat catalog.Catalog, exc execution.Execution, v vault.Vault) *arrowService {
	t.Helper()
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	return &arrowService{
		catalog:      cat,
		execution:    exc,
		asynxRuntime: axRuntime,
		vault:        v,
	}
}

func newTestServiceWithManifold(t *testing.T, cat catalog.Catalog, m manifoldIface) *arrowService {
	t.Helper()
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	return &arrowService{
		catalog:      cat,
		manifold:     m,
		execution:    &mockExecution{},
		asynxRuntime: axRuntime,
	}
}
```

Add a local interface alias in the test file to avoid importing the engine package:

```go
// manifoldIface matches manifold.Manifold for test wiring.
type manifoldIface interface {
	ResolveArrow(context.Context, domain.Namespace) (*domain.ArrowManifest, error)
	ResolveQuiver(context.Context, domain.Namespace) (*domain.QuiverManifest, error)
	ParseArrow([]byte) (*domain.ArrowManifest, error)
}
```

Add the actual test cases:

```go
// --- Seed ---

func TestSeed_InvalidNamespace_ReturnsError(t *testing.T) {
	svc := newTestServiceWithManifold(t, &mockCatalog{}, &mockManifold{})
	err := svc.Seed(context.Background(), "bad", []byte("yaml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestSeed_ManifoldParseError_ReturnsInvalidManifestError(t *testing.T) {
	m := &mockManifold{parseErr: errors.New("parse failed")}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, m)
	err := svc.Seed(context.Background(), "github.com/user/repo", []byte("bad"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidManifest)
}

func TestSeed_CatalogError_ReturnsError(t *testing.T) {
	manifest := makeTestManifest("arrow")
	m := &mockManifold{parseManifest: manifest}
	cat := &mockCatalog{addErr: apperrors.ErrAlreadyExists}
	svc := newTestServiceWithManifold(t, cat, m)
	err := svc.Seed(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.Error(t, err)
}

func TestSeed_Success_DelegatesToCatalogAddWithManifest(t *testing.T) {
	manifest := makeTestManifest("arrow")
	m := &mockManifold{parseManifest: manifest}
	cat := &mockCatalog{}
	svc := newTestServiceWithManifold(t, cat, m)
	err := svc.Seed(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
}

// --- ValidateManifest ---

func TestValidateManifest_ManifoldSuccess_ReturnsValidTrue(t *testing.T) {
	manifest := makeTestManifest("arrow")
	m := &mockManifold{parseManifest: manifest}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, m)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidateManifest_AssemblerErrors_ReturnsValidFalseWithErrors(t *testing.T) {
	asmErrs := assembler.AssemblerErrors{
		{Field: "lifecycle.install", Rule: "missing_pair", Message: "install requires uninstall"},
	}
	m := &mockManifold{parseErr: asmErrs}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, m)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "lifecycle.install", result.Errors[0].Field)
	assert.Equal(t, "missing_pair", result.Errors[0].Rule)
}

func TestValidateManifest_TranslatorError_ReturnsServiceError(t *testing.T) {
	m := &mockManifold{parseErr: errors.New("unknown schema")}
	svc := newTestServiceWithManifold(t, &mockCatalog{}, m)
	result, err := svc.ValidateManifest(context.Background(), "github.com/user/repo", []byte("yaml"))
	require.Error(t, err)
	assert.Nil(t, result)
}
```

Add the import for assembler package in `arrow_test.go`:
```go
"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
```

- [ ] **Step 5: Run tests to verify they fail**

```bash
go test ./internal/app/arrow/ -v -run "TestSeed|TestValidateManifest"
```

Expected: FAIL — compile error, methods not defined.

- [ ] **Step 6: Add `manifold` field to `arrowService` and implement the two methods in `arrow.go`**

Add `manifold` field to the struct (add import for `manifold` package):

```go
import (
    // existing imports...
    "github.com/rabbytesoftware/quiver/internal/engine/manifold"
    "github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
)

type arrowService struct {
    catalog      catalog.Catalog
    execution    execution.Execution
    asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
    vault        vault.Vault
    manifold     manifold.Manifold
}
```

Add to the `ArrowService` interface:

```go
Seed(
    ctx context.Context,
    ns domain.Namespace,
    data []byte,
) error
ValidateManifest(
    ctx context.Context,
    ns domain.Namespace,
    data []byte,
) (*ValidationResult, error)
```

Implement both methods on `arrowService`:

```go
func (svc *arrowService) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("seed arrow: %w", apperrors.ErrInvalidNamespace)
	}

	manifest, err := svc.manifold.ParseArrow(data)
	if err != nil {
		return fmt.Errorf("seed arrow: %w", apperrors.ErrInvalidManifest)
	}

	return svc.catalog.AddWithManifest(ctx, ns, manifest)
}

func (svc *arrowService) ValidateManifest(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) (*ValidationResult, error) {
	_, err := svc.manifold.ParseArrow(data)
	if err == nil {
		return &ValidationResult{Valid: true}, nil
	}

	var asmErrs assembler.AssemblerErrors
	if errors.As(err, &asmErrs) {
		errs := make([]ValidationError, len(asmErrs))
		for i, ae := range asmErrs {
			errs[i] = ValidationError{
				Field:   ae.Field,
				Rule:    ae.Rule,
				Message: ae.Message,
			}
		}
		return &ValidationResult{Valid: false, Errors: errs}, nil
	}

	return nil, fmt.Errorf("validate manifest: %w", err)
}
```

- [ ] **Step 7: Wire `manifold` in `builder.go`**

In the `Build()` method, update the `arrowService` construction:

```go
return &arrowService{
    catalog:      cat,
    execution:    exc,
    asynxRuntime: axRuntime,
    vault:        e.Vault,
    manifold:     e.Manifold,
}, nil
```

- [ ] **Step 8: Run service tests**

```bash
go test ./internal/app/arrow/... -v -run .
```

Expected: all PASS.

- [ ] **Step 9: Add `Seed` and `ValidateManifest` to `internal/api/mocks/arrow_service.go`**

Add fields to the struct and implement both methods:

```go
type ArrowService struct {
    // existing fields...
    SeedErr              error
    ValidateManifestResult *arrow.ValidationResult
    ValidateManifestErr    error
}

func (m *ArrowService) Seed(_ context.Context, _ domain.Namespace, _ []byte) error {
    return m.SeedErr
}

func (m *ArrowService) ValidateManifest(
    _ context.Context,
    _ domain.Namespace,
    _ []byte,
) (*arrow.ValidationResult, error) {
    return m.ValidateManifestResult, m.ValidateManifestErr
}
```

- [ ] **Step 10: Verify full app layer compiles**

```bash
go build ./internal/...
```

Expected: no errors.

- [ ] **Step 11: Commit**

```bash
git add internal/app/errors/errors.go internal/api/libs/apierr/mapper.go internal/app/arrow/ internal/api/mocks/arrow_service.go
git commit -m "feat(arrow): add Seed and ValidateManifest service methods"
```

---

## Task 5: API layer — DTO, handlers, routes

**Files:**
- Create: `internal/api/v0/dto/arrow_validation.go`
- Modify: `internal/api/v0/endpoints/arrows/handlers/handlers.go`
- Modify: `internal/api/v0/endpoints/arrows/handlers/handlers_test.go`
- Modify: `internal/api/v0/endpoints/arrows/routes.go`

- [ ] **Step 1: Create `internal/api/v0/dto/arrow_validation.go`**

```go
package dto

import "github.com/rabbytesoftware/quiver/internal/app/arrow"

// ValidationResultDTO is the response body for SEED /arrow/:ns/validate.
type ValidationResultDTO struct {
	Valid  bool                 `json:"valid"`
	Errors []ValidationErrorDTO `json:"errors,omitempty"`
}

// ValidationErrorDTO represents a single rule violation.
type ValidationErrorDTO struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ValidationResultDTOFrom maps the app-layer result to the API DTO.
func ValidationResultDTOFrom(r *arrow.ValidationResult) ValidationResultDTO {
	if len(r.Errors) == 0 {
		return ValidationResultDTO{Valid: r.Valid}
	}
	errs := make([]ValidationErrorDTO, len(r.Errors))
	for i, e := range r.Errors {
		errs[i] = ValidationErrorDTO{
			Field:   e.Field,
			Rule:    e.Rule,
			Message: e.Message,
		}
	}
	return ValidationResultDTO{Valid: r.Valid, Errors: errs}
}
```

- [ ] **Step 2: Write failing handler tests in `handlers_test.go`**

Add to the `setup` function to also register SEED routes:

```go
func setup(svc *mocks.ArrowService) (*arrows.Handlers, *gin.Engine) {
    h := arrows.New(svc)
    r := gin.New()
    r.UseRawPath = true
    r.UnescapePathValues = true
    r.POST("/v0/arrow/:ns", h.Add)
    r.PATCH("/v0/arrow/:ns", h.Update)
    r.DELETE("/v0/arrow/:ns", h.Remove)
    r.GET("/v0/arrow", h.List)
    r.GET("/v0/arrow/:ns", h.GetDetail)
    r.POST("/v0/arrow/:ns/:method", h.Execute)
    r.Handle("SEED", "/v0/arrow/:ns", h.Seed)
    r.Handle("SEED", "/v0/arrow/:ns/validate", h.Validate)
    return h, r
}
```

Add new test cases:

```go
// --- Seed handler ---

func TestSeed_Created(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("SEED", encodedNS, bytes.NewBufferString("manifest: arrow@v0"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
	assertSuccess(t, w.Body.Bytes())
}

func TestSeed_ServiceError_InvalidManifest(t *testing.T) {
	svc := &mocks.ArrowService{SeedErr: apperrors.ErrInvalidManifest}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("SEED", encodedNS, bytes.NewBufferString("bad yaml"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestSeed_EmptyBody_ReturnsError(t *testing.T) {
	svc := &mocks.ArrowService{SeedErr: apperrors.ErrInvalidManifest}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("SEED", encodedNS, nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// --- Validate handler ---

func TestValidate_ValidManifest_Returns200WithValidTrue(t *testing.T) {
	svc := &mocks.ArrowService{
		ValidateManifestResult: &arrow.ValidationResult{Valid: true},
	}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("SEED", encodedNS+"/validate", bytes.NewBufferString("manifest: arrow@v0"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data struct {
			Valid   bool `json:"valid"`
			Errors  []struct {
				Field string `json:"field"`
			} `json:"errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Data.Valid)
	assert.Empty(t, body.Data.Errors)
}

func TestValidate_InvalidManifest_Returns200WithValidFalseAndErrors(t *testing.T) {
	svc := &mocks.ArrowService{
		ValidateManifestResult: &arrow.ValidationResult{
			Valid: false,
			Errors: []arrow.ValidationError{
				{Field: "lifecycle.install", Rule: "missing_pair", Message: "install requires uninstall"},
			},
		},
	}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("SEED", encodedNS+"/validate", bytes.NewBufferString("manifest: arrow@v0"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data struct {
			Valid  bool `json:"valid"`
			Errors []struct {
				Field   string `json:"field"`
				Rule    string `json:"rule"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.False(t, body.Data.Valid)
	require.Len(t, body.Data.Errors, 1)
	assert.Equal(t, "lifecycle.install", body.Data.Errors[0].Field)
	assert.Equal(t, "missing_pair", body.Data.Errors[0].Rule)
}

func TestValidate_ServiceError_Returns500(t *testing.T) {
	svc := &mocks.ArrowService{ValidateManifestErr: errors.New("translator failed")}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("SEED", encodedNS+"/validate", bytes.NewBufferString("not yaml"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
```

Add missing imports to `handlers_test.go`:
```go
import (
    "bytes"
    "encoding/json"
    "errors"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/rabbytesoftware/quiver/internal/api/mocks"
    arrows "github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/arrows/handlers"
    "github.com/rabbytesoftware/quiver/internal/app/arrow"
    apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
    "github.com/rabbytesoftware/quiver/internal/domain"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
```

- [ ] **Step 3: Run handler tests to verify they fail**

```bash
go test ./internal/api/v0/endpoints/arrows/... -v -run "TestSeed|TestValidate"
```

Expected: FAIL — compile error, `h.Seed` and `h.Validate` not defined.

- [ ] **Step 4: Add `Seed` and `Validate` handler methods to `handlers.go`**

Add after the existing `Execute` method:

```go
func (h *Handlers) Seed(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "failed to read body", string(ns))
		return
	}

	if err := h.svc.Seed(c.Request.Context(), ns, body); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}

	libs.WriteMutationOK(c, http.StatusCreated, string(ns))
}

func (h *Handlers) Validate(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "failed to read body", string(ns))
		return
	}

	result, err := h.svc.ValidateManifest(c.Request.Context(), ns, body)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}

	libs.WriteQueryOK(c, apidto.ValidationResultDTOFrom(result))
}
```

Add `"io"` to the imports in `handlers.go`.

- [ ] **Step 5: Register SEED routes in `routes.go`**

Add after the existing `rg.POST("/arrow/:ns/:method", h.Execute)` line:

```go
rg.Handle("SEED", "/arrow/:ns", h.Seed)
rg.Handle("SEED", "/arrow/:ns/validate", h.Validate)
```

- [ ] **Step 6: Run all handler tests**

```bash
go test ./internal/api/v0/endpoints/arrows/... -v -run .
```

Expected: all PASS.

- [ ] **Step 7: Run the full test suite**

```bash
make test
```

Expected: all PASS, no regressions.

- [ ] **Step 8: Commit**

```bash
git add internal/api/v0/dto/arrow_validation.go internal/api/v0/endpoints/arrows/ internal/api/libs/apierr/
git commit -m "feat(api): add SEED /arrow/:ns and SEED /arrow/:ns/validate endpoints"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** All 5 components from the spec are covered (Assembler, Manifold, Catalog, ArrowService, API).
- [x] **No placeholders:** Every step has complete code or an exact command.
- [x] **Type consistency:** `AssemblerError` fields (`Field`, `Rule`, `Message`) are the same across Tasks 1, 2, 4, and 5. `ValidationResult`/`ValidationError` match across Tasks 4 and 5. `AddWithManifest` signature is consistent between Tasks 3 and 4.
- [x] **Backward compat:** `errors.Is(err, ErrInvalidManifest)` still works via `AssemblerError.Unwrap()`. `ValidateArrow` return type is still `error`. All existing tests unmodified except for added import in `assembler_test.go`.
- [x] **Mock completeness:** `mocks.Manifold`, `mocks.ArrowService` both updated in their respective tasks.
- [x] **Builder wired:** `e.Manifold` injected into `arrowService` in Task 4 Step 7.
