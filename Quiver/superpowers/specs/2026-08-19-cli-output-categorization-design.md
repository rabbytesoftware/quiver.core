# CLI Output Categorization & Formatting Design

**Date:** 2026-08-19  
**Status:** Draft (awaiting review)  
**Scope:** Standardize CLI payload shapes and output rendering across all commands

---

## 1. Overview

The Quiver CLI currently has inconsistent output shapes and rendering strategies across commands. This design introduces **five standardized output categories**, each with:

- A **typed payload wrapper** (DTO contract)
- **JSON/YAML rendering rules** (standard serialization of the wrapper)
- **Table rendering rules** (domain-specific, handcrafted per category)
- **Examples** for each command type

Every command will emit one of these five categories. This ensures:
- Testability: validate wrapper contracts, not raw data
- Extensibility: metadata (count, timestamp) fits naturally in the wrapper
- Polish: table rendering is semantic, not auto-derived
- Parsability: JSON/YAML consumers always see the same structure

---

## 2. Output Categories & DTO Contracts

### 2.1 Discovery

**Purpose:** List or search arrows and collections. May be filtered by pattern.

**DTO Contract:**

```go
// DiscoveryResult wraps a list of discovery query results.
type DiscoveryResult struct {
    // Items are the arrows and/or collections matching the query.
    Items []DiscoveryItem `json:"items" yaml:"items"`
    // Total is the count of items in the result.
    Total int `json:"total" yaml:"total"`
    // Query is the pattern used (empty = all items).
    Query string `json:"query" yaml:"query"`
}

// DiscoveryItem is a union of Arrow and Collection, tagged by Kind.
type DiscoveryItem struct {
    Kind       string                   `json:"kind" yaml:"kind"` // "arrow" or "collection"
    Arrow      *ArrowListItemDTO        `json:"arrow,omitempty" yaml:"arrow,omitempty"`
    Collection *CollectionListItemDTO   `json:"collection,omitempty" yaml:"collection,omitempty"`
}
```

**Commands in this category:**
- `quiver list [pattern]` — list all arrows and collections, optionally filtered
- `quiver search <pattern>` — search for arrows and collections by pattern

**Table Rendering:**

```
Arrows:
  NAMESPACE               NAME              STATE       INSTALLED
  github.com/user/repo    repo              ready       yes
  github.com/user/repo2   repo2             absent      no

Collections:
  NAME           ARROWS
  my-tools       5
  devops-stack   12
```

- Two subsections: Arrows and Collections
- Arrow rows: namespace, name, state, installed Y/N
- Collection rows: name, arrow count
- If query was applied, show "Filtered: 8 of 20 arrows" at the top

**JSON/YAML Examples:**

```json
{
  "items": [
    {
      "kind": "arrow",
      "arrow": {
        "namespace": "github.com/user/repo",
        "name": "repo",
        "state": "ready",
        "user_installed": true
      }
    },
    {
      "kind": "collection",
      "collection": {
        "name": "my-tools",
        "arrows": 5
      }
    }
  ],
  "total": 2,
  "query": ""
}
```

---

### 2.2 Observation

**Purpose:** Query current state of arrows or runtimes. No mutation. Multiple results possible.

**DTO Contract:**

```go
// ObservationResult wraps one or more state snapshots.
type ObservationResult struct {
    // Items are the observed states (arrows, runtimes, or mixed).
    Items []ObservedItem `json:"items" yaml:"items"`
    // SnapshotTime is when this observation was captured.
    SnapshotTime string `json:"snapshot_time" yaml:"snapshot_time"` // RFC3339
}

// ObservedItem is a union, tagged by Kind.
type ObservedItem struct {
    Kind    string             `json:"kind" yaml:"kind"` // "arrow" or "runtime"
    Arrow   *ArrowStateDTO     `json:"arrow,omitempty" yaml:"arrow,omitempty"`
    Runtime *ArrowRuntimeDTO   `json:"runtime,omitempty" yaml:"runtime,omitempty"`
}

// ArrowStateDTO is a lightweight arrow snapshot (namespace, name, state).
type ArrowStateDTO struct {
    Namespace string `json:"namespace" yaml:"namespace"`
    Name      string `json:"name" yaml:"name"`
    State     string `json:"state" yaml:"state"`
}
```

**Commands in this category:**
- `quiver status <namespace>` — show current state of one arrow
- `quiver ps` — list all running runtimes with their state
- `quiver observe` (if added) — query state of one or more arrows

**Table Rendering:**

For `status`:
```
ARROW:        github.com/user/repo
STATE:        running
INSTALLED:    2026-08-15 14:22:00
LAST_RUN:     2026-08-19 09:15:30
```

For `ps`:
```
NAMESPACE               METHOD    STATE     PID      STARTED
github.com/user/repo    run       running   1234     2026-08-19 09:15:30
github.com/user/repo2   install   running   1235     2026-08-19 09:20:00
```

**JSON/YAML Example:**

```json
{
  "items": [
    {
      "kind": "runtime",
      "runtime": {
        "namespace": "github.com/user/repo",
        "method": "run",
        "state": "running",
        "pid": 1234,
        "started_at": "2026-08-19T09:15:30Z"
      }
    }
  ],
  "snapshot_time": "2026-08-19T10:30:45Z"
}
```

---

### 2.3 Lifecycle

**Purpose:** Execute an action (install, run, stop, update, uninstall) and report progress + outcome.

**DTO Contract:**

```go
// LifecycleOutcome wraps the result of a lifecycle operation.
type LifecycleOutcome struct {
    // Subject is the namespace being operated on.
    Subject string `json:"subject" yaml:"subject"`
    // Action is the operation performed (install, run, stop, update, uninstall).
    Action string `json:"action" yaml:"action"`
    // Success is true if the operation completed without error.
    Success bool `json:"success" yaml:"success"`
    // Steps are the execution steps and their outcomes.
    Steps []StepRecord `json:"steps" yaml:"steps"`
    // FinalState is the arrow's state after completion (e.g., "ready", "running").
    FinalState string `json:"final_state" yaml:"final_state"`
    // Timestamp is when the operation completed.
    Timestamp string `json:"timestamp" yaml:"timestamp"` // RFC3339
}

// StepRecord is a single step's name, state, and duration.
type StepRecord struct {
    Name      string `json:"name" yaml:"name"`
    State     string `json:"state" yaml:"state"` // "pending", "running", "done", "failed"
    Duration  string `json:"duration" yaml:"duration"` // e.g., "3.2s"
    Error     string `json:"error,omitempty" yaml:"error,omitempty"`
}
```

**Commands in this category:**
- `quiver install <namespace>` — install an arrow
- `quiver run <namespace> [method]` — run an arrow or method
- `quiver stop <namespace>` — stop a running arrow
- `quiver update <namespace>` — update an arrow to a new version
- `quiver uninstall <namespace>` — uninstall an arrow

**Table Rendering:**

During execution (while still streaming):
```
installing github.com/user/repo
  ⠋ fetch manifest
  ✓ resolve dependencies
  ⠙ run install script
```

On completion:
```
✓ install github.com/user/repo (4.2s)

  fetch manifest     ✓ 0.3s
  resolve dependencies ✓ 0.5s
  run install script ✓ 3.4s

State: ready
```

On failure:
```
✗ install github.com/user/repo (2.1s)

  fetch manifest     ✓ 0.3s
  resolve dependencies ✓ 0.5s
  run install script ✗ 1.3s
    error: exit code 1 (see logs for details)

State: absent
```

**JSON/YAML Example:**

```json
{
  "subject": "github.com/user/repo",
  "action": "install",
  "success": true,
  "steps": [
    {
      "name": "fetch manifest",
      "state": "done",
      "duration": "0.3s"
    },
    {
      "name": "resolve dependencies",
      "state": "done",
      "duration": "0.5s"
    },
    {
      "name": "run install script",
      "state": "done",
      "duration": "3.4s"
    }
  ],
  "final_state": "ready",
  "timestamp": "2026-08-19T10:30:45Z"
}
```

---

### 2.4 Mutation

**Purpose:** Catalog operation that changes state (add, remove, refresh). Single subject, simple confirmation.

**DTO Contract:**

```go
// MutationResult wraps the outcome of a catalog mutation.
type MutationResult struct {
    // Action is the operation performed (add, remove, refresh).
    Action string `json:"action" yaml:"action"`
    // Subject is the namespace or context name being mutated.
    Subject string `json:"subject" yaml:"subject"`
    // Success is true if the mutation succeeded.
    Success bool `json:"success" yaml:"success"`
    // Message is a human-readable summary (for errors, the error text).
    Message string `json:"message" yaml:"message"`
    // Timestamp is when the mutation was completed.
    Timestamp string `json:"timestamp" yaml:"timestamp"` // RFC3339
}
```

**Commands in this category:**
- `quiver arrow add <namespace>` — register an arrow
- `quiver arrow remove <namespace>` — remove an arrow
- `quiver arrow refresh <namespace>` — re-fetch an arrow's manifest
- `quiver collection follow <namespace>` — follow a collection
- `quiver collection unfollow <namespace>` — unfollow a collection
- `quiver context add <name> <server>` — save a context
- `quiver context remove <name>` — remove a context

**Table Rendering:**

Single line, suitable for piped consumption:
```
added github.com/user/repo
```

On error:
```
failed to remove github.com/user/repo: still has dependents
```

(The message field contains the error text if success=false.)

**JSON/YAML Example:**

```json
{
  "action": "add",
  "subject": "github.com/user/repo",
  "success": true,
  "message": "registered arrow and resolved manifest",
  "timestamp": "2026-08-19T10:30:45Z"
}
```

---

### 2.5 Info Query

**Purpose:** Fetch detailed information about a single subject (arrow, collection, or context). Rich, structured output.

**DTO Contract:**

```go
// InfoResult wraps detailed information about a subject.
type InfoResult struct {
    // Kind identifies what kind of subject (arrow, collection, context).
    Kind string `json:"kind" yaml:"kind"` // "arrow", "collection", "context"
    // Subject is the detailed information (arrow: ArrowDetailDTO, etc.).
    Subject any `json:"subject" yaml:"subject"`
    // RelatedInfo is optional context (e.g., methods available, dependents).
    RelatedInfo map[string]any `json:"related_info,omitempty" yaml:"related_info,omitempty"`
}

// ArrowDetailDTO is the full arrow with manifest and runtime state.
type ArrowDetailDTO struct {
    Namespace   string                      `json:"namespace" yaml:"namespace"`
    Name        string                      `json:"name" yaml:"name"`
    Description string                      `json:"description" yaml:"description"`
    State       string                      `json:"state" yaml:"state"`
    InstalledAt string                      `json:"installed_at,omitempty" yaml:"installed_at,omitempty"`
    Version     string                      `json:"version" yaml:"version"`
    Tags        []string                    `json:"tags" yaml:"tags"`
    Manifest    *domain.Arrow               `json:"manifest" yaml:"manifest"`
    Runtime     *ArrowRuntimeDTO            `json:"runtime,omitempty" yaml:"runtime,omitempty"`
}
```

**Commands in this category:**
- `quiver show <namespace>` — show full details of an arrow (manifest + state + runtime)
- `quiver methods <namespace>` — list available methods and their signatures
- `quiver collection show <name>` — show collection details
- `quiver context show <name>` — show context configuration (mask secrets)

**Table Rendering:**

For `show`:
```
ARROW:          github.com/user/repo
NAME:           repo
DESCRIPTION:    A sample arrow for demonstration
STATE:          ready
INSTALLED:      2026-08-15 14:22:00
VERSION:        v1.2.0
TAGS:           demo, testing

MANIFEST:
  Variables:
    NAME          TYPE      REQUIRED  DEFAULT
    LOG_LEVEL     string    no        info
    PORT          int       no        8080

  Lifecycle:
    install       install_script.sh
    run           run_script.sh
    stop          stop_script.sh
    update        update_script.sh
    uninstall     uninstall_script.sh

  Methods:
    health        check service health
    logs          dump recent logs

RUNTIME (currently running):
  Method:        run
  PID:           1234
  Started:       2026-08-19 09:15:30
  Duration:      1h 15m
```

For `methods`:
```
ARROW:  github.com/user/repo

AVAILABLE METHODS:
  install      (lifecycle)
  run          (lifecycle)
  stop         (lifecycle)
  update       (lifecycle)
  uninstall    (lifecycle)
  health       (custom) — check service health
  logs         (custom) — dump recent logs
```

**JSON/YAML Example:**

```json
{
  "kind": "arrow",
  "subject": {
    "namespace": "github.com/user/repo",
    "name": "repo",
    "description": "A sample arrow",
    "state": "ready",
    "version": "v1.2.0",
    "installed_at": "2026-08-15T14:22:00Z",
    "tags": ["demo", "testing"],
    "manifest": { /* full Arrow domain object */ },
    "runtime": {
      "method": "run",
      "pid": 1234,
      "started_at": "2026-08-19T09:15:30Z"
    }
  },
  "related_info": {
    "methods": [
      { "name": "health", "description": "check service health" },
      { "name": "logs", "description": "dump recent logs" }
    ]
  }
}
```

---

## 3. Command-to-Category Mapping

| Command | Category | Payload Type | Notes |
|---------|----------|---|---|
| `list [pattern]` | Discovery | DiscoveryResult | Multi-section table (arrows + collections) |
| `search <pattern>` | Discovery | DiscoveryResult | Same as list, filter applied |
| `arrow add <ns>` | Mutation | MutationResult | Single-line: "added <ns>" |
| `arrow remove <ns>` | Mutation | MutationResult | Single-line: "removed <ns>" |
| `arrow refresh <ns>` | Mutation | MutationResult | Single-line: "refreshed <ns>" |
| `show <ns>` | Info Query | InfoResult | Multi-section formatted view |
| `methods <ns>` | Info Query | InfoResult | Method list + descriptions |
| `status <ns>` | Observation | ObservationResult | Single arrow state snapshot |
| `ps` | Observation | ObservationResult | Multi-row runtime list |
| `install <ns>` | Lifecycle | LifecycleOutcome | Progress stream → outcome |
| `run <ns> [method]` | Lifecycle | LifecycleOutcome | Progress stream → outcome |
| `stop <ns>` | Lifecycle | LifecycleOutcome | Progress stream → outcome |
| `update <ns>` | Lifecycle | LifecycleOutcome | Progress stream → outcome |
| `uninstall <ns>` | Lifecycle | LifecycleOutcome | Progress stream → outcome |
| `collection follow <ns>` | Mutation | MutationResult | Single-line: "followed <ns>" |
| `collection unfollow <ns>` | Mutation | MutationResult | Single-line: "unfollowed <ns>" |
| `context add <name>` | Mutation | MutationResult | Single-line: "added context <name>" |
| `context remove <name>` | Mutation | MutationResult | Single-line: "removed context <name>" |
| `context show <name>` | Info Query | InfoResult | Context details (mask secrets) |
| `context list` | Observation | ObservationResult | List of context names + servers |

---

## 4. Output Rendering Rules

### 4.1 Format Matrix

| Category | Table | JSON | YAML | Piped Behavior |
|----------|-------|------|------|---|
| **Discovery** | Multi-section; arrows + collections subsections | Wrapper + items array | Wrapper + items array | Items one per line if --output=jsonl? |
| **Observation** | Multi-row or key-value depending on count | Wrapper + items array | Wrapper + items array | Same structure |
| **Lifecycle** | Live stream during execution; summary on completion | Wrapper + steps array | Wrapper + steps array | Whole result at end |
| **Mutation** | Single line: "action subject" | Wrapper object | Wrapper object | Single line (same as table) |
| **Info Query** | Multi-section formatted view | Wrapper + nested subject | Wrapper + nested subject | Same structure |

### 4.2 Terminal vs. Piped Behavior

**Table format (`--output` unset or `--output=table`):**
- **On TTY:** Render interactive table (with spinners/colors for lifecycle progress)
- **Piped:** Render static text version (plain ASCII tables, no ANSI codes)

**JSON/YAML formats (`--output=json` or `--output=yaml`):**
- **On TTY or piped:** Identical output (always the wrapped structure)
- **No colors or interactive elements**

### 4.3 Error Handling

When a command fails:

1. **Mutation or Observation:** Return the same wrapper with `Success: false` and an error message in the `Message` field.
2. **Lifecycle:** Return LifecycleOutcome with `Success: false`, failed steps marked with state "failed", and error text in the step's Error field.
3. **Discovery or Info Query:** If the operation fails (e.g., arrow not found), return a standard error (not wrapped) with exit code 1.

Exit codes:
- **0:** Success
- **1:** Operation failed (error message in stderr)
- **2:** Usage error (bad arguments)
- **3:** Connection error (daemon unreachable)

---

## 5. Testing Strategy

### 5.1 Payload Validation Tests

For each command, write a test that:
1. Runs the command
2. Parses the output in JSON format
3. Asserts the wrapper type and all required fields are present
4. Validates field values (e.g., `action == "add"`, `total >= 0`)
5. Confirms JSON/YAML roundtrip (unmarshal, remarshal, compare)

**Example test:**

```go
func TestArrowAddCmd_ValidPayload(t *testing.T) {
    // Setup: run `quiver arrow add github.com/user/repo` and capture JSON output
    output := captureOutput(t, "arrow", "add", "github.com/user/repo", "-o", "json")
    
    var result MutationResult
    require.NoError(t, json.Unmarshal([]byte(output), &result))
    
    // Assertions
    assert.Equal(t, "add", result.Action)
    assert.Equal(t, "github.com/user/repo", result.Subject)
    assert.True(t, result.Success)
    assert.NotEmpty(t, result.Timestamp)
}
```

### 5.2 Table Rendering Tests

For each command, write a test that:
1. Runs the command with no `--output` flag (or `--output=table`)
2. Captures the piped text output
3. Asserts the output contains expected elements (column headers, data rows, etc.)
4. Uses snapshot testing for complex multi-section renders (e.g., `show`)

**Example test:**

```go
func TestArrowAddCmd_TableOutput(t *testing.T) {
    output := captureOutput(t, "arrow", "add", "github.com/user/repo")
    
    // Assert single-line format
    assert.Equal(t, "added github.com/user/repo\n", output)
}

func TestShowCmd_TableOutput(t *testing.T) {
    output := captureOutput(t, "show", "github.com/user/repo")
    
    // Assert multi-section structure
    assert.Contains(t, output, "ARROW:")
    assert.Contains(t, output, "STATE:")
    assert.Contains(t, output, "MANIFEST:")
    
    // Snapshot test for exact format
    require.Truef(t, snapshotMatches(t, output, "show_table_output"), 
        "table output snapshot mismatch")
}
```

### 5.3 Category Contract Tests

Write a suite of tests that validates each category's DTO contract across all commands in that category:

```go
// Test that all Discovery commands return a DiscoveryResult with items array and total count
func TestDiscoveryCategory_ContractCompliance(t *testing.T) {
    for _, cmd := range [][]string{
        {"list"},
        {"list", "pattern"},
        {"search", "pattern"},
    } {
        t.Run(strings.Join(cmd, "/"), func(t *testing.T) {
            output := captureOutput(t, append(cmd, "-o", "json")...)
            
            var result DiscoveryResult
            require.NoError(t, json.Unmarshal([]byte(output), &result))
            
            // Contract assertions
            require.NotNil(t, result.Items, "Items must not be nil")
            assert.GreaterOrEqual(t, result.Total, len(result.Items))
            // ... other contract checks
        })
    }
}
```

---

## 6. Migration Path

### Phase 1: Define DTOs (in progress)
- [ ] Create new DTO types in `internal/api/v0/dto/` (5 wrapper types + all supporting types)
- [ ] Add `CheckPayload()` validation method to each wrapper type
- [ ] Add tests for DTO serialization (JSON/YAML roundtrip)

### Phase 2: Update Commands (batch by category)
- [ ] **Mutation category:** update `arrow add/remove/refresh`, `collection follow/unfollow`, `context add/remove`
  - Return MutationResult instead of bare string
  - Update table rendering to single-line format
- [ ] **Observation category:** update `status`, `ps`
  - Return ObservationResult wrapper
  - Refine table rendering per command
- [ ] **Discovery category:** update `list`, `search`
  - Return DiscoveryResult wrapper
  - Standardize table sections
- [ ] **Lifecycle category:** update `install`, `run`, `stop`, `update`, `uninstall`
  - Return LifecycleOutcome with steps array
  - Ensure table rendering shows live progress
- [ ] **Info Query category:** update `show`, `methods`, `context show`
  - Return InfoResult wrapper
  - Standardize detail formatting

### Phase 3: Add Tests (parallel with Phase 2)
- [ ] Write payload validation tests for each command
- [ ] Write table rendering snapshot tests
- [ ] Write category contract compliance tests
- [ ] Verify 95% coverage on new code

### Phase 4: Integration & Polish
- [ ] End-to-end flow tests: run each command, validate output, confirm exit codes
- [ ] CLI Result Map update: mark each command as ✓ (payload standardized)
- [ ] Documentation: update CLI user manual with output examples

---

## 7. File Structure Changes

**New files to create:**

```
internal/api/v0/dto/
  discovery_result.go        // DiscoveryResult + DiscoveryItem DTOs
  observation_result.go      // ObservationResult + ObservedItem DTOs
  lifecycle_outcome.go       // LifecycleOutcome + StepRecord DTOs
  mutation_result.go         // MutationResult DTO
  info_result.go             // InfoResult DTO

internal/cli/commands/
  *_test.go                  // Payload validation tests for each command
  
internal/cli/tui/
  table_renderer.go          // Table rendering rules per category (optional consolidation)
```

**Modified files:**

```
internal/cli/commands/
  arrow.go                   // arrow add/remove/refresh return MutationResult
  collection.go              // collection follow/unfollow return MutationResult
  context.go                 // context add/remove return MutationResult
  discovery.go               // list/search return DiscoveryResult
  lifecycle.go               // install/run/stop/update/uninstall return LifecycleOutcome
  observe.go                 // status/ps return ObservationResult
  system.go                  // show/methods/context show return InfoResult
```

---

## 8. Examples: Before & After

### Example 1: `arrow add` Command

**Before:**
```
$ quiver arrow add github.com/user/repo
added github.com/user/repo

$ quiver arrow add github.com/user/repo -o json
(no structured output; fails or returns error message)
```

**After:**
```
$ quiver arrow add github.com/user/repo
added github.com/user/repo

$ quiver arrow add github.com/user/repo -o json
{
  "action": "add",
  "subject": "github.com/user/repo",
  "success": true,
  "message": "registered arrow and resolved manifest",
  "timestamp": "2026-08-19T10:30:45Z"
}

$ quiver arrow add github.com/user/repo -o yaml
action: add
subject: github.com/user/repo
success: true
message: registered arrow and resolved manifest
timestamp: "2026-08-19T10:30:45Z"
```

### Example 2: `list` Command

**Before:**
```
$ quiver list
(table rendering varies, no consistent structure)

$ quiver list -o json
{
  "arrows": [ /* array of ArrowListItemDTO */ ],
  "collections": [ /* array of CollectionListItemDTO */ ]
}
```

**After:**
```
$ quiver list
Arrows:
  NAMESPACE               NAME              STATE       INSTALLED
  github.com/user/repo    repo              ready       yes
  github.com/user/repo2   repo2             absent      no

Collections:
  NAME           ARROWS
  my-tools       5
  devops-stack   12

$ quiver list -o json
{
  "items": [
    {
      "kind": "arrow",
      "arrow": {
        "namespace": "github.com/user/repo",
        "name": "repo",
        "state": "ready",
        "user_installed": true
      }
    },
    {
      "kind": "collection",
      "collection": {
        "name": "my-tools",
        "arrows": 5
      }
    }
  ],
  "total": 2,
  "query": ""
}
```

### Example 3: `install` Command

**Before:**
```
$ quiver install github.com/user/repo
(interactive TTY with steps, but output on piped TTY/JSON is undefined)
```

**After:**
```
$ quiver install github.com/user/repo
installing github.com/user/repo
  ⠋ fetch manifest
  ✓ resolve dependencies
  ⠙ run install script
  (after completion)
✓ install github.com/user/repo (4.2s)

$ quiver install github.com/user/repo -o json
{
  "subject": "github.com/user/repo",
  "action": "install",
  "success": true,
  "steps": [
    { "name": "fetch manifest", "state": "done", "duration": "0.3s" },
    { "name": "resolve dependencies", "state": "done", "duration": "0.5s" },
    { "name": "run install script", "state": "done", "duration": "3.4s" }
  ],
  "final_state": "ready",
  "timestamp": "2026-08-19T10:30:45Z"
}
```

---

## 9. Success Criteria

1. ✓ All commands emit one of the five category types
2. ✓ JSON/YAML output is always the wrapped structure (never bare data)
3. ✓ Table rendering is domain-specific and polished (not auto-derived)
4. ✓ All commands have payload validation tests (95% coverage)
5. ✓ CLI Result Map updated with output examples
6. ✓ User manual includes output format reference

---

## 10. Open Questions & Deferred Decisions

1. **Pagination for large lists:** Should DiscoveryResult include `page`, `page_size`, `has_next`? Deferred until we see real usage.
2. **Error details in JSON:** Should failed steps include `stderr` or just `error` message? Recommend: just `error` (full logs via separate endpoint).
3. **Backward compatibility:** Should we version the DTO wrappers (e.g., v0.1, v0.2)? Recommend: no—we're in pre-release, breaking changes are acceptable.
4. **Single-line JSON:** Should mutations also support `--output=jsonl` (one JSON object per line)? Recommend: defer to a future enhancement.

---

## Appendix: DTO Pseudocode Summary

```go
type DiscoveryResult struct {
    Items []DiscoveryItem
    Total int
    Query string
}

type ObservationResult struct {
    Items        []ObservedItem
    SnapshotTime string
}

type LifecycleOutcome struct {
    Subject    string
    Action     string
    Success    bool
    Steps      []StepRecord
    FinalState string
    Timestamp  string
}

type MutationResult struct {
    Action    string
    Subject   string
    Success   bool
    Message   string
    Timestamp string
}

type InfoResult struct {
    Kind        string
    Subject     any
    RelatedInfo map[string]any
}
```

Each DTO includes both `json` and `yaml` tags on all fields.

---

**This design is ready for review. Please provide feedback before implementation begins.**
