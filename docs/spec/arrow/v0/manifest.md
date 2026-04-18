# Arrow Manifest — `arrow@v0` Spec

This document is the normative specification for the `arrow@v0` manifest format after the
targets refactor. It supersedes the manifest description in `docs/spec/entities.md` (§1 Arrow
manifest format), which is retained only as a historical overview. All tooling must conform to
this document.

Cross-references: [manifold.md](../../manifold.md) · [deptree.md](../../deptree.md) ·
[vault.md](../../vault.md) · [wizard.md](../../wizard.md) · [netbridge.md](../../netbridge.md)

---

## 1. Overview

An Arrow manifest is a YAML file that describes a piece of software Quiver can install, run, and
manage. The `arrow@v0` format introduced targets — a first-class mechanism for expressing
platform-specific recipes — alongside the pre-existing `Overrideable[T]` scalar-override
mechanism and a new `base:` inheritance key.

### 1.1 What changed from pre-refactor v0

| Aspect | Pre-refactor | This spec |
|--------|-------------|-----------|
| Cross-platform mechanism | `Overrideable[T]` on step fields, with bare OS keys (`linux`) | `targets:` for structural divergence; `Overrideable[T]` narrowed to scalar variance within glob targets, keys are full `GOOS/GOARCH` patterns |
| OS key format | Bare family names: `linux`, `windows`, `macos` | Full `GOOS/GOARCH` patterns: `linux/amd64`, `windows/*`, `*` |
| Platform selection | Single shared recipe shape for all platforms | Per-target recipe, selected at runtime by `GOOS/GOARCH` |
| `requirements`, `dependencies` | Top-level fields | Moved inside each target; `dependencies:` renamed to `tools:` |
| `lifecycle`, `methods` | Top-level fields | Moved inside each target |
| Service vs. package | Implicit from execute/stop presence | Still implicit — unchanged |
| Sharing between platforms | Not supported | `base:` with abstract targets (`_`-prefixed keys) |
| Methods contract | Shared across all platforms | Per-target; each target declares only the methods that apply to it |
| Variables | Top-level only | Top-level only — global contract, no per-target variables |
| Arrow relationships | Single `dependencies:` list, no runtime semantics | `tools:` (install-time tools) + `services:` (runtime services) + `exports:` (stable named interface) |

v0 is still in active development; no migration shim is required. Old-shape manifests (lacking
`targets:`) must be rejected at parse time with a clear error pointing to this spec.
See [§13 Migration note](#13-migration-note).

### 1.2 Progressive complexity tiers

Not every Arrow needs the full feature set. The format is designed so simple cases stay simple:

| Tier | Pattern | Typical use case |
|------|---------|-----------------|
| **1 — Universal** | Single `*` target, no `base:`, no Overrideable | Cross-platform, identical steps everywhere |
| **2 — Platform-aware** | Multiple targets, optional `base:`, optional Overrideable | Platform-specific steps or downloads |
| **3 — Multi-Arrow system** | `exports:`, `services:`, Overrideable on exports | Arrows that coordinate with other Arrows |

Arrows that fit Tier 1 look almost identical to the pre-refactor format — just wrapped in a
`targets: "*":` block. Developers should start at the lowest tier that covers their needs and
only reach for higher-tier features when the use case requires it.

---

## 2. Top-level structure

```yaml
schema: "arrow@v0"           # required — must be exactly this string

metadata:                    # required
  name: string               # required — human-readable display name
  description: string        # required — short one-line description
  version: string            # required — software version (semver recommended)
  license: string            # optional — SPDX identifier
  url: string                # optional — homepage or documentation URL
  quiver: string             # optional — Quiver namespace this Arrow belongs to
  maintainers:               # optional — list of maintainer objects
    - name: string           # required within entry
      email: string          # optional
      url: string            # optional
  credits:                   # optional — attribution to upstream authors
    - name: string
      email: string          # optional
      url: string            # optional
  media:                     # optional
    icon: string             # URL to icon image
    banner: string           # URL to banner image
  tags:                      # optional — free-form strings for store discovery
    - string

variables:                   # optional — manifest-level user-configurable parameters
  - name: string             # required — identifier used in ${VAR} interpolation
    type: string             # required — one of: string, number, boolean, select
    default: any             # required — default value
    description: string      # optional
    sensitive: boolean       # optional — display hint only; not a security boundary (default: false)
    values: [string]         # optional — allowed values; required when type is select
    min: number              # optional — minimum value; only when type is number
    max: number              # optional — maximum value; only when type is number

netbridge:                   # optional — declared port intent
  - name: string             # required — identifier used in ${PORT} interpolation
    default: integer         # required — default port number
    protocol: string         # required — one of: tcp, udp, tcp/udp
    sensitive: boolean       # optional (default: false)
    required: boolean        # optional (default: true)

targets:                     # required — platform-specific recipes (see §3)
  <target-key>:
    base: string             # optional — parent target key (see §4)
    requirements:            # optional — minimum system resources
      cpu_cores: integer
      ram_gb: integer
      disk_gb: integer
    tools:                   # optional — install-time only: tools/libraries this Arrow calls
      - string               # namespace, optionally versioned: "github.com/foo/bar@v1.2.3"
    services:                # optional — runtime: service Arrows that must run alongside
      - string               # namespace, optionally versioned: "github.com/foo/bar@v2.0.0"
    exports:                 # optional — named values this Arrow exposes to dependents
      <export-name>: string  # plain string or Overrideable map (same rules as step scalar fields)
    lifecycle:               # required in every concrete (non-abstract) target
      install:   [steps]     # required
      update:    [steps]     # optional — in-place update; falls back to uninstall+reinstall if absent
      execute:   [steps]     # optional — must be paired with stop
      stop:      [steps]     # optional — must be paired with execute
      uninstall: [steps]     # required (may be empty list [])
    methods:                 # optional — developer-defined custom actions
      <method-name>:
        available_in: [string]   # states where this method can be invoked
        steps: [steps]
```

The top-level `variables:` and `netbridge:` sections are the Arrow's public contract — the
form the user fills in before install and the ports Netbridge allocates. Both are global:
they apply uniformly across all platforms and are never declared inside a target. Per-platform
scalar variance in step commands is handled by Overrideable fields (§5), not by variables.

---

## 3. Targets

### 3.1 Target key forms

Every key in `targets:` is one of three forms:

| Form | Example | Description |
|------|---------|-------------|
| Exact | `linux/amd64` | Matches exactly one concrete `GOOS/GOARCH` |
| Glob | `linux/*`, `*/arm64`, `*` | Standard glob — `*` matches any single path segment |
| Abstract | `_common`, `_unix` | Key starts with `_`; never selected at runtime |

Concrete `GOOS/GOARCH` values Quiver recognises (from `internal/domain/os.go`):

```
linux/amd64   linux/arm64
windows/amd64 windows/arm64
darwin/amd64  darwin/arm64
```

Any other `GOOS/GOARCH` (e.g. `freebsd/amd64`) is unrecognised at parse time but may be
targeted by glob keys; Quiver will fail at target-selection time if the runtime OS is
unrecognised.

### 3.2 Target selection and compilation

Quiver never selects targets at execution time. Instead, all targets are compiled once at
`arrow.Add` time (and again on `update_manifest`) by running `SelectTarget` for each of the
six known concrete `GOOS/GOARCH` values:

```
for each os in domain.AllOS():
    target, err := manifest.SelectTarget(os)
    if err == nil:
        compiledTargets[os] = target    // base: chain flattened, Overrideable resolved
```

If zero OS values compile successfully, the Arrow is rejected with a hard error:

> `Arrow resolves to no supported platform. All 6 OS/arch combinations failed to match any concrete target.`

The resulting `map[OS]ResolvedTarget` is stored in the Arrow aggregate. At execution time,
the app layer does a single map lookup:

```
target, ok := arrow.CompiledTargets[domain.CurrentOS()]
if !ok → "this Arrow does not support your platform"
```

**Platform compatibility is implicit** — the keys of `CompiledTargets` are the supported
platform set. No separate declaration is needed.

**Validation response format:** the add/validation endpoint returns the full platform
compatibility breakdown alongside any errors:

```json
{
  "supported_platforms": ["linux/amd64", "linux/arm64", "darwin/amd64"],
  "unsupported_platforms": ["windows/amd64", "windows/arm64", "darwin/arm64"],
  "errors": [...]
}
```

Partial coverage (some platforms supported, others not) is permitted — the developer opted
into a platform-limited Arrow. Zero supported platforms is a hard error.

`SelectTarget(os)` applies the following rules:

1. Collect all non-abstract target keys.
2. Test each key against `os` via glob matching.
3. Rank matches by specificity:
   - Exact (`linux/amd64`) — rank 3
   - One wildcard, non-catch-all (`linux/*`, `*/arm64`) — rank 2
   - Catch-all (`*`) — rank 1
4. Highest rank wins. Ties are a parse-time error — no two equally-specific keys may both
   match the same concrete `GOOS/GOARCH`.
5. Flatten the `base:` chain of the winning key (§4).
6. Resolve all Overrideable fields against `os` (§5).
7. Return the fully resolved `ResolvedTarget`.

### 3.3 Domain model impact

The compilation model affects two domain types:

**`ArrowManifest`** — raw, as parsed from YAML. Stored in the Vault for display and
re-compilation. Contains `Targets map[string]Target` with Overrideable fields intact.

**`Arrow` aggregate** — event-sourced. Stores `CompiledTargets map[OS]ResolvedTarget`
alongside display metadata (`Name`, `Description`, `Version`, `License`, `Tags`,
`Maintainers`). The raw manifest is not in the aggregate.

**`ResolvedTarget`** — fully flattened, plain scalar steps for one concrete OS. Contains
`Requirements`, `Tools`, `Services`, `Exports` (`map[string]string`), `Netbridge`,
`Lifecycle`, `Methods`. No Overrideable fields, no `base:` references. Export values are
static strings — Overrideable platform variance resolved, no `${VAR}` tokens remaining.
Variables are read directly from the manifest-level `variables:` list at resolution time —
they are not copied into `ResolvedTarget`. This is what the runner, installer, and
catalog dep-checker read at runtime.

### 3.4 Abstract targets

A target whose key starts with `_` is abstract:

- It is never selected at runtime and never appears in `CompiledTargets`.
- It may only be referenced via `base:`.
- It may omit `lifecycle:` entirely (useful as a base that only provides `execute`/`stop`/`methods`).
- Referencing an abstract target anywhere other than `base:` is a parse-time error.

---

## 4. `base:`

### 4.1 Purpose

`base:` allows a concrete (or abstract) target to inherit all fields from a single parent
target and selectively override what differs. It replaces copy-paste for platforms that share
most of their recipe.

```yaml
targets:
  _common:
    lifecycle:
      execute:
        - type: run
          command: ./server
          title: Starting server
          timeout: 10s
          exit_on_failure: true
      stop:
        - type: signal
          signal: graceful
          timeout: 10s
          exit_on_failure: false
      uninstall: []

  "linux/*":
    base: _common
    lifecycle:
      install:            # added — not in _common
        - type: run
          command: ./setup.sh
          title: Installing
          timeout: 5m
          exit_on_failure: true
      # execute, stop, uninstall inherited from _common
```

### 4.2 Override rules

After the `base:` chain is resolved, a child target is flattened by merging into its parent.
The child always has precedence — whatever the child defines for a field, it **overrides** that
field from the base entirely. Fields left undefined in the child are inherited unchanged.

| Field category | Behavior |
|----------------|----------|
| Scalars (any step scalar field) | Child overrides parent |
| Maps (`lifecycle:`, `methods:`, `requirements:`) | Recursive key-by-key override. A child that defines `lifecycle.install` overrides only that hook; the parent's `execute`, `stop`, `uninstall` are inherited. A child that adds one method overrides that method; sibling methods from parent are inherited. |
| Step lists (any `[steps]` field) | Child overrides parent wholesale. No per-step merge — the entire list is replaced. |
| `tools:` | Child overrides parent wholesale. |
| `services:` | Child overrides parent wholesale. |
| `exports:` (map) | Key-by-key override. Child export entries override parent entries with the same name; unique entries from both sides are kept. |

The rule is intentional and consistent: **what you write, you own**. If a child target defines
a lifecycle hook, it owns that hook entirely — the base version is gone. If it does not define
the hook, the base version applies unchanged. Developers should not place step lists in an
abstract base if concrete targets will need to modify them.

### 4.3 Constraints

- **Single parent only.** `base:` takes one key — multi-parent inheritance is not supported in v0.
- **No cycles.** A → B → A is a parse-time error.
- **Parent must exist.** Referencing a missing target key in `base:` is a parse-time error.
- **Abstract targets only via `base:`.** Using an `_`-prefixed key outside `base:` is a parse-time error.

---

## 5. Overrideable fields

### 5.1 Purpose and scope

`Overrideable[T]` handles scalar variance within an otherwise-identical recipe. Its natural home
is inside glob targets, where the containing target matches multiple concrete `GOOS/GOARCH`
values and a single scalar (typically a download URL or binary name) differs per arch.

An Overrideable field may appear on any step type. The overrideable scalar fields are:

| Step type | Overrideable fields |
|-----------|---------------------|
| `run` | `command`, `timeout`, `exit_on_failure` |
| `fetch` | `url`, `to`, `timeout`, `exit_on_failure` |
| `signal` | `signal`, `timeout`, `exit_on_failure` |

`type` is never overrideable — a step's type is fixed. `title` is never overrideable — titles
are display-only and do not vary per platform.

### 5.2 YAML representation

A scalar field is either a plain scalar (no override) or a mapping with `GOOS/GOARCH`-pattern
keys (Overrideable). The two forms are mutually exclusive on a given field:

```yaml
# Plain scalar — identical on all platforms the target matches
command: ./mytool

# Overrideable — value varies per arch
url:
  linux/amd64: https://example.com/tool-linux-amd64.tar.gz
  linux/arm64: https://example.com/tool-linux-arm64.tar.gz

# Overrideable with default — catch-all for arches not listed explicitly
command:
  default: ./mytool
  "windows/*": '.\mytool.exe'
```

The `default:` key inside an Overrideable map acts as a fallback for any concrete `GOOS/GOARCH`
that matches the containing target but is not covered by an explicit key. It is required
whenever the containing target can match `GOOS/GOARCH` values not listed explicitly by other
keys — omitting it in that case is a parse-time error (Rule 5).

### 5.3 Key format

Every key in an Overrideable map must be one of:

- An exact `GOOS/GOARCH` string: `linux/amd64`, `darwin/arm64`, etc.
- A glob pattern: `linux/*`, `*/arm64`, `*`
- The special word `default`

Bare OS family names (`linux`, `windows`, `darwin`) are not valid.

Resolution uses the same specificity ranking as target selection (§3.2). Among all keys that
match the runtime `GOOS/GOARCH`, the most specific wins.

### 5.4 Coverage rule

For every Overrideable map in a target, every concrete `GOOS/GOARCH` that the containing
target can match must be reachable — via an exact key, a matching glob, or `default`.

When the target key is a glob (e.g. `linux/*`), a `default:` key or a `linux/*` glob key in
the Overrideable map satisfies coverage for all current and future Linux arches. If neither is
present, every concrete `linux/<arch>` in the domain enum must be covered by an exact key.

Unreachable concrete `GOOS/GOARCH` values are a parse-time error — there is no silent fallback.

### 5.5 Only on scalar fields

Overrideable applies exclusively to the step-level scalar fields listed in §5.1. It does **not**
apply to step lists, `tools:`, `requirements:`, top-level `variables:`, or `netbridge:`.

Structural divergence belongs in `targets:`; scalar variance belongs in Overrideable.

---

## 6. Arrow relationships

Arrows can relate to other Arrows in two distinct ways. Both are declared per-target, since
relationship needs can vary per platform.

### 6.1 `tools:` — install-time tools and libraries

`tools:` lists Arrows that must be installed before this Arrow installs. Their binaries
and files are available via exports (§6.3) or `${namespace.INSTALL_PATH}`. They are never
started or stopped by this Arrow's lifecycle — they are tools, not services.

```yaml
tools:
  - github.com/valve/steamcmd
```

### 6.2 `services:` — runtime service dependencies

`services:` lists service Arrows that must be running alongside this Arrow during execution.
Declaring an Arrow in `services:` implies `tools:` — Quiver also installs it before
this Arrow installs. The same Arrow must not appear in both `tools:` and `services:`
(validation rule 10).

```yaml
services:
  - github.com/char2cs/myapp/database
```

**Quiver behavior:**

| Event | Behavior |
|-------|----------|
| Install | All `services:` Arrows are installed first (same as `tools:`) |
| Execute | Each `services:` Arrow is started in dependency order if not already running |
| Stop | Each `services:` Arrow is stopped in reverse order, unless another running Arrow also requires it |

If a required Arrow is already running when this Arrow executes, Quiver skips starting it and
does not stop it when this Arrow stops — it is not this Arrow's responsibility.

### 6.3 `exports:` — named values exposed to dependents

`exports:` is how an Arrow exposes a stable interface to Arrows that depend on it. Instead of
dependents reaching into `INSTALL_PATH` and guessing file locations, the Arrow declares named
exports — named values that dependents reference by name. This decouples the Arrow's internal
file layout from its public interface.

Export values are Overrideable static strings (same platform-key rules as step scalar fields —
§5). **`${VAR}` interpolation is not allowed inside export values** — exports must be fully
static so they can be resolved at compile time (SelectTarget).

```yaml
# steamcmd.yaml
targets:
  "*":
    exports:
      steamcmd:
        default: ./steamcmd.sh    # relative to steamcmd's INSTALL_PATH
        "windows/*": ./steamcmd.exe

      python: /usr/bin/python3    # absolute — passed through as-is
```

Dependents reference exports via `${namespace.EXPORT_NAME}`:

```yaml
# cs2.yaml
tools:
  - github.com/valve/steamcmd

targets:
  "linux/*":
    lifecycle:
      install:
        - type: run
          command: ${github.com/valve/steamcmd.steamcmd} +app_update 730 +quit
          title: Installing CS2 via SteamCMD
          timeout: 30m
          exit_on_failure: true
```

**Relative export values are anchored automatically.** When the variable resolver encounters
`${namespace.EXPORT_NAME}` and the export value is a relative path (starts with `./`), it
anchors it against the dependency's `INSTALL_PATH` before substitution — producing an absolute
path the shell can execute. Absolute export values are passed through as-is. The consumer
never needs to manually combine `${namespace.INSTALL_PATH}` with an export reference.

Export values are compiled as part of `SelectTarget` — Quiver resolves only the Overrideable
keys (choosing the right platform variant). The resolved export map (`map[string]string`) is
part of `ResolvedTarget` and is fully available at add time.

**`${namespace.INSTALL_PATH}` is a built-in export**, always available for every Arrow
regardless of whether it defines an `exports:` section. Use it only when you need the raw
install directory path — for referencing binaries, prefer named exports.

**Export reference validation** happens at add time: when Quiver compiles an Arrow's targets,
every `${namespace.EXPORT_NAME}` in step fields is checked against the dependency's compiled
exports for the same OS. If the export does not exist, it is a parse-time error — the same
category as a missing dependency Arrow. See validation rule 9.

---

## 7. Lifecycle

### 7.1 Hooks

Each target's `lifecycle:` section defines up to four hooks:

| Hook | Pair | State transition | Required |
|------|------|-----------------|----------|
| `install` | install/uninstall | `(not installed) → ready` | Yes in every concrete target |
| `update` | — (standalone) | `ready → updating → ready` | No — falls back to uninstall+reinstall |
| `uninstall` | install/uninstall | `* → removed` | Yes (may be empty list `[]`) |
| `execute` | execute/stop | `ready → running` | No — omit for packages |
| `stop` | execute/stop | `running → ready (via stopping)` | No — required only if execute is defined |

The install/uninstall pair is always implicit. Even if both hooks have empty step lists,
Quiver injects Step 0 (dependency resolution) at the start of every install execution. The
Wizard never sees the `dependencies` synthetic step — it is managed by the app layer. See
[deptree.md](../../deptree.md) §Call Site for the full install flow.

`update` is standalone — it has no required pair. It runs in-place inside the existing
installation directory, preserving user data and runtime artifacts. If an arrow omits
`update:`, Quiver falls back to uninstall + reinstall when `quiver update` is invoked —
this is destructive and should be documented in the arrow's README. `update` is only
invocable from `ready` state; a running service must be stopped before updating.

### 7.2 Working directory

All steps execute with `${INSTALL_PATH}` as the working directory. Relative paths in `run`
commands (`./mytool`, `./setup.sh`) and `fetch` destinations (`to: ./binary`) are relative to
`INSTALL_PATH`. This is guaranteed by the Wizard before passing steps to the OS.

### 7.3 Pairing rules

- `execute` and `stop` must always appear together within the same target. One without the other
  is a parse-time error (applies after `base:` flattening).
- `install` and `uninstall` must always appear together within the same target. One without the
  other is a parse-time error (applies after flattening).
- `update` is standalone — it requires neither a pair nor any other hook to be present alongside
  it. It may appear in any concrete target regardless of kind (package or service).

### 7.4 Service vs. package (kind inference)

Quiver infers the Arrow kind from the presence of execute/stop:

- **Package** (no running process): no `execute`/`stop` in any concrete target.
- **Service** (long-running process): `execute`/`stop` in every concrete target.
- **Mixed** (some targets have execute/stop, others don't): parse-time error (validation rule 3).

There is no explicit `kind:` field — the structure is the declaration.

### 7.5 Step types

Every step in a lifecycle hook (or method) has a `type` field. Valid types:

| `type` | Purpose | Required fields | Optional fields | Overrideable fields |
|--------|---------|----------------|-----------------|---------------------|
| `run` | Execute a shell command | `command` | `elevated` | `command`, `elevated`, `timeout`, `exit_on_failure` |
| `fetch` | Download a remote file | `url`, `to` | `checksum` | `url`, `to`, `checksum`, `timeout`, `exit_on_failure` |
| `signal` | Send a cross-platform shutdown signal to the managed process | `signal` | — | `signal`, `timeout`, `exit_on_failure` |

All steps also accept:
- `title` (string, optional) — human-readable label shown in the UI
- `timeout` (string, optional) — maximum duration; must be in the form `<number>s` (seconds) or `<number>m` (minutes), e.g. `30s`, `5m`, `300s`. Hours and compound durations are not valid.
- `exit_on_failure` (boolean, optional, default `true`) — whether to abort the execution on failure

**`run` elevated execution:** when `elevated: true` is set, Quiver runs the command with
elevated privileges — `sudo` on Linux and macOS, a UAC-elevated process on Windows. Default
is `false`. `elevated` is Overrideable, so platforms that handle their own elevation internally
can opt out: `elevated: { "linux/*": true, "windows/*": false }`.

Elevation is platform-specific in implementation:

| Platform | Mechanism | Requirement |
|----------|-----------|-------------|
| Linux | `sudo <command>` | Quiver's process user must have passwordless sudo for the command, or the system must allow `pkexec` |
| macOS | `sudo <command>` | Same as Linux, or Quiver invokes `osascript` to present an admin dialog |
| Windows | ShellExecuteEx `runas` | Triggers a UAC prompt to the user; Quiver spawns an elevated child process |

Because Quiver runs as a headless service, `sudo` on Linux/macOS requires either `NOPASSWD`
sudoers configuration for the Quiver process user or a platform-specific privilege escalation
helper. Manifest authors should document this requirement in their Arrow's README. If elevation
fails (user denies UAC, sudo not configured), the step fails and `exit_on_failure` governs the
rest of the execution.

**`signal` termination signals:** the `signal` field is an enum — not a raw POSIX signal name.
Valid values and their platform mappings:

| Value | Linux / macOS | Windows |
|-------|--------------|---------|
| `graceful` | `SIGTERM` — request orderly shutdown | `Stop-Process` — allows the process to clean up |
| `kill` | `SIGKILL` — immediate forced termination | `taskkill /F` — immediate forced termination |
| `interrupt` | `SIGINT` — keyboard interrupt equivalent | `GenerateConsoleCtrlEvent` — Ctrl+C equivalent |

`signal` is Overrideable — different enum values may be used per platform if needed.

**`fetch` checksum verification:** the optional `checksum` field accepts a string in the form
`<algorithm>:<hex-digest>`, e.g. `sha256:abc123...`. Supported algorithms: `sha256`, `sha512`.
When present, Quiver verifies the downloaded file against the digest before proceeding. A
mismatch is treated as a step failure — subject to `exit_on_failure`. Omitting `checksum` is
valid; manifest authors are encouraged to include it for any binary download.

The `dependencies` step type is synthetic — injected by Quiver as Step 0 of every install
execution. It must never appear in manifests.

### 7.6 Shell execution semantics

All `run` step commands are passed to a platform shell. The shell is fixed per platform:

| Platform | Shell | Invocation |
|----------|-------|-----------|
| Linux | `/bin/sh` | `/bin/sh -c "<command>"` |
| macOS | `/bin/sh` | `/bin/sh -c "<command>"` |
| Windows | PowerShell | `powershell -NoProfile -Command "<command>"` |

Shell features are available on each platform — `&&`, `|`, redirects, and variable expansion work
within their respective shell. Commands that must run on multiple platforms should use only
syntax valid in both `/bin/sh` and PowerShell, or be split into separate targets.

`${VAR}` interpolation by Quiver happens **before** the command is passed to the shell —
the shell never sees the raw `${VAR}` token, only the resolved value.

---

## 8. Methods

Methods are developer-defined custom actions. Unlike lifecycle hooks they do not transition the
Arrow between states; they are actions the user can invoke when the Arrow is in a specific state.

### 8.1 Structure

```yaml
methods:
  <method-name>:
    available_in: [string]   # required — enum[]; valid values: ready | running
    steps: [steps]           # required
```

### 8.2 Per-target autonomy

Each target declares only the methods that are meaningful for it. There is no cross-target
method contract — a method that exists on `linux/*` does not need to exist on `windows/*` and
vice versa. If a method has no meaningful implementation on a platform, omit it from that
target entirely.

`available_in` is also per-target. A `restart` method may gate on `[running]` on Linux and
`[running, paused]` on Windows — the platform decides when invocation is valid.

### 8.3 `available_in` gating

Valid states for `available_in`: `ready`, `running`. Any other value is a parse-time error
(validation rule 13).

The Quiver runtime enforces `available_in` at invocation time — invoking a method from a state
not in its `available_in` list returns an error to the caller.

---

## 9. Variable resolution pipeline

All `${VAR}` references in step fields are resolved by the app layer after target compilation
and before steps are passed to the Wizard. Resolution uses a 6-layer priority stack; later
layers override earlier ones.

| Priority | Source | Example |
|----------|--------|---------|
| 1 (lowest) | Built-in runtime variables | `${INSTALL_PATH}`, `${ARROW_NAMESPACE}`, `${PLATFORM}` |
| 2 | Dependency exports and built-in variables | `${github.com/valve/steamcmd.steamcmd}`, `${github.com/valve/steamcmd.INSTALL_PATH}` |
| 3 | Manifest-level `variables:` defaults | `variables[].default` |
| 4 | Netbridge port allocations | Port `name` → allocated port number as string |
| 5 | Stored variables | `LastReturn.Variables` from most recent completed execution |
| 6 (highest) | User-provided overrides | Key-value pairs from the HTTP request body |

**Dependency reference syntax:** `${namespace.NAME}` resolves by looking up `NAME` in the
dependency's compiled exports first, then in the built-in variables (`INSTALL_PATH`,
`ARROW_NAMESPACE`, `PLATFORM`). Named exports take precedence over built-ins if they share a
name (which they should not — export names colliding with built-in names are a validation
error).

When the resolved export value is a relative path (starts with `./`), the variable resolver
anchors it against the dependency's `INSTALL_PATH` automatically, producing an absolute path.
Absolute export values are substituted as-is. This means `${github.com/valve/steamcmd.steamcmd}`
resolves to a fully usable path in all cases — the consumer never constructs paths manually.

**Built-in variables** always available in step interpolation:

| Variable | Description |
|----------|-------------|
| `${INSTALL_PATH}` | Home directory for this Arrow (provided by `vault.GetArrow`/`vault.PutArrow`) |
| `${ARROW_NAMESPACE}` | This Arrow's full namespace |
| `${PLATFORM}` | Current platform as `GOOS/GOARCH` string (e.g. `linux/amd64`) |

`${PLATFORM}` reflects the full `GOOS/GOARCH` value — not a bare OS family name.

**Variable persistence across the Arrow's lifetime:** variables are persisted by the
`arrowRuntime` aggregate throughout the entire lifecycle of an installed Arrow. A variable
resolved during `install` is available — via layer 5 (stored variables) — when `uninstall`,
`execute`, `stop`, or any method runs later. User-provided overrides (layer 6) are available
only when explicitly passed to the invocation.

The fully resolved steps and variable map are passed to `wizard.Execute` via `ExecutionRequest`.
See [wizard.md](../../wizard.md) §ExecutionRequest for the full contract.

---

## 10. Validation rules

All rules apply at parse time (manifest load), after `base:` chains are fully flattened.

### Rule 1 — Install/uninstall present

Every non-abstract target must define both `install` and `uninstall`. `uninstall` may be an
empty list. One without the other is an error.

> Error: `target "linux/*": install defined without uninstall`

### Rule 2 — Execute/stop paired

Within each target (after flattening), `execute` and `stop` must both be present or both absent.

> Error: `target "windows/amd64": execute defined without stop`

### Rule 3 — Service/package consistency

Either every non-abstract target defines `execute`+`stop`, or none does. Mixed is an error.

> Error: `mixed kind: target "linux/*" defines execute/stop but target "darwin/*" does not`

### Rule 4 — `base:` integrity

- No cycles in the `base:` chain.
- Every referenced parent key must exist in `targets:`.
- Abstract target keys (`_`-prefixed) must not appear outside `base:`.

> Error: `base: cycle detected: _unix → _common → _unix`
> Error: `target "linux/*" base: "_base" which does not exist`
> Error: `abstract target "_common" used outside base:`

### Rule 5 — Overrideable coverage

For every Overrideable field map in a target, every concrete `GOOS/GOARCH` the containing
target can match must be reachable via an exact key, a glob key, or `default`.

> Error: `target "linux/*", install step 0, field "url": no Overrideable key covers linux/arm64 — add an explicit key or a "default:" fallback`

### Rule 6 — Variable reference hygiene

After target flattening, every `${VAR}` reference in step fields must resolve to a known
variable name, a `netbridge:` port name, a dependency built-in, or a platform built-in.

> Error: `target "linux/*", install step 1: unresolved variable ${TARBALL_URL}`

### Rule 7 — Overrideable key validity

Every key in an Overrideable map must be `default`, an exact `GOOS/GOARCH` string, or a valid
glob pattern. Bare OS family names are not valid.

> Error: `target "linux/*", install step 0, url: invalid Overrideable key "linux" — use a full GOOS/GOARCH pattern`

### Rule 8 — Ambiguous target keys

No two non-abstract target keys may have equal specificity for the same concrete `GOOS/GOARCH`.

> Error: `targets "linux/*" and "*/amd64" are equally specific for linux/amd64 — make one more specific or merge them`

### Rule 9 — Export reference hygiene

Every `${namespace.EXPORT_NAME}` reference in step fields must resolve to either a declared
export or a built-in variable (`INSTALL_PATH`, `ARROW_NAMESPACE`, `PLATFORM`) on the
referenced dependency's compiled target for the same OS. The dependency must be listed in
`tools:` or `services:`. Validated at add time if the dependency Arrow is already in
the catalog; hard error at install time otherwise. Export names must not collide with built-in
variable names.

Export values themselves must be static strings — `${VAR}` tokens inside an export value are
a parse-time error.

> Error: `target "linux/*", install step 0: ${github.com/valve/steamcmd.steamcmd} — export "steamcmd" not found on github.com/valve/steamcmd for linux/amd64`
> Error: `target "linux/*": ${github.com/valve/steamcmd.INSTALL_PATH} — github.com/valve/steamcmd is not declared in tools or services`
> Error: `target "*", exports.steamcmd: ${VAR} interpolation is not allowed in export values — use a static string or relative path`

### Rule 10 — `services:` / `tools:` exclusivity

The same Arrow namespace must not appear in both `tools:` and `services:` within the
same target. `services:` already implies install-time dependency.

> Error: `target "linux/*": github.com/char2cs/myapp/database appears in both tools and services — remove it from tools`

### Rule 11 — At least one supported platform

After compiling all six concrete `GOOS/GOARCH` values, at least one must produce a valid
`ResolvedTarget`. An Arrow that resolves to zero platforms is rejected.

> Error: `Arrow resolves to no supported platform. All 6 OS/arch combinations failed to match any concrete target. Add at least one concrete target (e.g. "*", "linux/*", "linux/amd64").`

### Rule 12 — `timeout:` format

Every `timeout` value must match `<positive-integer>s` (seconds) or `<positive-integer>m`
(minutes). Bare numbers, hour suffixes, compound durations, and non-numeric prefixes are not valid.

> Error: `target "linux/*", install step 2: invalid timeout "2h" — use seconds (e.g. "120s") or minutes (e.g. "2m")`
> Error: `target "linux/*", install step 0: invalid timeout "30" — suffix required: "30s" or "30m"`

### Rule 13 — `available_in:` values

Every entry in an `available_in` list must be one of the valid Arrow states: `ready`, `running`.
Any other value is a parse-time error.

> Error: `method "restart", available_in: "stopped" is not a valid state — valid values: ready, running`

---

## 11. Use cases

The four examples below are the canonical worked manifests for `arrow@v0`. Each is complete and
valid under this spec — every step includes all fields. Methods are declared only where they
make sense for a given platform; no `unsupported` entries are needed.

---

### 11.1 Universal static package — Claude Skill (WASM plugin)

**Tier 1.** A WASM plugin that runs identically on all platforms. Single `"*"` target. No
execute/stop — package kind.

```yaml
schema: "arrow@v0"

metadata:
  name: anthropic.claude-skill-web-search
  description: Web search skill for Claude Code
  version: 1.0.0
  license: Apache-2.0
  quiver: github.com/anthropic/claude-skills
  url: https://anthropic.com/claude-code
  maintainers:
    - name: Anthropic
      url: https://anthropic.com
  tags:
    - claude
    - skill
    - ai

variables:
  - name: SEARCH_PROVIDER
    type: select
    default: "default"
    values: ["default", "google", "bing"]
    description: Search provider backend
    sensitive: false

  - name: SEARCH_API_KEY
    type: string
    default: ""
    description: API key for the selected search provider
    sensitive: true

targets:
  "*":
    lifecycle:
      install:
        - type: fetch
          url: https://skills.anthropic.com/web-search/v1.0.0/skill.wasm
          to: ./skill.wasm
          title: Downloading web search skill
          timeout: 5m
          exit_on_failure: true

        - type: run
          command: ./skill.wasm --setup --provider ${SEARCH_PROVIDER}
          title: Configuring skill
          timeout: 2m
          exit_on_failure: true

      uninstall: []

    methods:
      update:
        available_in: [ready]
        steps:
          - type: fetch
            url: https://skills.anthropic.com/web-search/latest/skill.wasm
            to: ./skill.wasm
            title: Downloading latest skill version
            timeout: 5m
            exit_on_failure: true

          - type: run
            command: ./skill.wasm --setup --provider ${SEARCH_PROVIDER}
            title: Reconfiguring skill
            timeout: 2m
            exit_on_failure: true
```

---

### 11.2 Single-platform service — Linux game server

**Tier 2.** A Linux-only game server. `linux/*` target with Overrideable URLs per arch. Service
kind. Demonstrates per-arch URL variance within a glob target and Netbridge port allocation.

```yaml
schema: "arrow@v0"

metadata:
  name: char2cs.myserver
  description: My awesome Linux game server
  version: 1.0.0
  license: MIT
  quiver: github.com/char2cs/gaming.quiver
  maintainers:
    - name: char2cs
      email: me@char2cs.net

variables:
  - name: MAX_PLAYERS
    type: number
    default: "16"
    description: Maximum concurrent players
    min: 1
    max: 128
    sensitive: false

netbridge:
  - name: GAME_PORT
    default: 27015
    protocol: tcp/udp
    sensitive: false
    required: true

targets:
  "linux/*":
    requirements:
      cpu_cores: 2
      ram_gb: 4
      disk_gb: 20

    tools:
      - github.com/char2cs/gaming.quiver/base-libs

    lifecycle:
      install:
        - type: fetch
          url:
            linux/amd64: https://releases.myserver.io/v1.0.0/myserver-linux-amd64.tar.gz
            linux/arm64: https://releases.myserver.io/v1.0.0/myserver-linux-arm64.tar.gz
          to: ./myserver.tar.gz
          title: Downloading server binary
          timeout: 10m
          exit_on_failure: true

        - type: run
          command: tar -xzf ./myserver.tar.gz
          title: Extracting server
          timeout: 5m
          exit_on_failure: true

        - type: run
          command: chmod +x ./myserver
          title: Setting executable bit
          timeout: 10s
          exit_on_failure: true

      execute:
        - type: run
          command: ./myserver --port ${GAME_PORT} --maxplayers ${MAX_PLAYERS}
          title: Starting game server
          timeout: 30s
          exit_on_failure: true

      stop:
        - type: signal
          signal: graceful
          timeout: 30s
          exit_on_failure: false

      uninstall:
        - type: run
          command: rm -f ./myserver ./myserver.tar.gz
          title: Removing server binary
          timeout: 30s
          exit_on_failure: false

    methods:
      update:
        available_in: [ready]
        steps:
          - type: fetch
            url:
              linux/amd64: https://releases.myserver.io/latest/myserver-linux-amd64.tar.gz
              linux/arm64: https://releases.myserver.io/latest/myserver-linux-arm64.tar.gz
            to: ./myserver.tar.gz
            title: Downloading server update
            timeout: 10m
            exit_on_failure: true

          - type: run
            command: tar -xzf ./myserver.tar.gz
            title: Extracting update
            timeout: 5m
            exit_on_failure: true

          - type: run
            command: chmod +x ./myserver
            title: Setting executable bit
            timeout: 10s
            exit_on_failure: true
```

---

### 11.3 Cross-platform app, fully divergent — Firefox

**Tier 2.** Firefox uses `tar.bz2` + `chmod` on Linux, `hdiutil` / `.dmg` on macOS, and a
silent MSI installer on Windows. The uninstall steps are also platform-specific. Three
self-contained targets with no shared abstract base. Each target declares only the methods that
make sense for it — `set-default-browser` appears on Linux and macOS but not Windows (use OS
Settings instead); `clear-windows-registry` appears only on Windows.

```yaml
schema: "arrow@v0"

metadata:
  name: mozilla.firefox
  description: Mozilla Firefox web browser
  version: "130.0"
  license: MPL-2.0
  url: https://www.mozilla.org/firefox/
  quiver: github.com/rabbytesoftware/quiver.experiments
  maintainers:
    - name: Rabbyte Software
      url: https://rabbyte.com
  tags:
    - browser
    - web

variables:
  - name: FIREFOX_PROFILE
    type: string
    default: default
    description: Firefox profile name to create and use
    sensitive: false

targets:
  "linux/*":
    requirements:
      cpu_cores: 2
      ram_gb: 2
      disk_gb: 3

    lifecycle:
      install:
        - type: fetch
          url:
            linux/amd64: https://download.mozilla.org/?product=firefox-130.0&os=linux64&lang=en-US
            linux/arm64: https://download.mozilla.org/?product=firefox-130.0&os=linux64-aarch64&lang=en-US
          to: ./firefox.tar.bz2
          title: Downloading Firefox
          timeout: 15m
          exit_on_failure: true

        - type: run
          command: tar -xjf ./firefox.tar.bz2
          title: Extracting Firefox
          timeout: 5m
          exit_on_failure: true

        - type: run
          command: ./firefox/firefox --createprofile ${FIREFOX_PROFILE}
          title: Creating Firefox profile
          timeout: 1m
          exit_on_failure: true

      execute:
        - type: run
          command: ./firefox/firefox --profile ${FIREFOX_PROFILE}
          title: Launching Firefox
          timeout: 15s
          exit_on_failure: true

      stop:
        - type: signal
          signal: graceful
          timeout: 10s
          exit_on_failure: false

      uninstall:
        - type: run
          command: rm -rf ./firefox ./firefox.tar.bz2
          title: Removing Firefox
          timeout: 1m
          exit_on_failure: false

    methods:
      update:
        available_in: [ready]
        steps:
          - type: fetch
            url:
              linux/amd64: https://download.mozilla.org/?product=firefox-latest&os=linux64&lang=en-US
              linux/arm64: https://download.mozilla.org/?product=firefox-latest&os=linux64-aarch64&lang=en-US
            to: ./firefox-update.tar.bz2
            title: Downloading Firefox update
            timeout: 15m
            exit_on_failure: true

          - type: run
            command: tar -xjf ./firefox-update.tar.bz2
            title: Extracting Firefox update
            timeout: 5m
            exit_on_failure: true

          - type: run
            command: rm -f ./firefox-update.tar.bz2
            title: Cleaning up update archive
            timeout: 10s
            exit_on_failure: false

      set-default-browser:
        available_in: [ready]
        steps:
          - type: run
            command: xdg-settings set default-web-browser firefox.desktop
            title: Setting Firefox as default browser
            timeout: 30s
            exit_on_failure: false

  "windows/*":
    requirements:
      cpu_cores: 2
      ram_gb: 2
      disk_gb: 3

    lifecycle:
      install:
        - type: fetch
          url:
            windows/amd64: https://download.mozilla.org/?product=firefox-130.0&os=win64&lang=en-US
            windows/arm64: https://download.mozilla.org/?product=firefox-130.0&os=win64-aarch64&lang=en-US
          to: ./firefox-setup.exe
          title: Downloading Firefox installer
          timeout: 15m
          exit_on_failure: true

        - type: run
          command: '.\firefox-setup.exe /S /InstallDirectoryPath="${INSTALL_PATH}\firefox"'
          title: Installing Firefox silently
          timeout: 10m
          exit_on_failure: true

        - type: run
          command: '.\firefox\firefox.exe --createprofile ${FIREFOX_PROFILE}'
          title: Creating Firefox profile
          timeout: 1m
          exit_on_failure: true

      execute:
        - type: run
          command: '.\firefox\firefox.exe --profile ${FIREFOX_PROFILE}'
          title: Launching Firefox
          timeout: 15s
          exit_on_failure: true

      stop:
        - type: run
          command: taskkill /IM firefox.exe /F
          title: Stopping Firefox
          timeout: 10s
          exit_on_failure: false

      uninstall:
        - type: run
          command: '.\firefox\uninstall\helper.exe /S'
          title: Uninstalling Firefox
          timeout: 5m
          exit_on_failure: false

    methods:
      update:
        available_in: [ready]
        steps:
          - type: fetch
            url:
              windows/amd64: https://download.mozilla.org/?product=firefox-latest&os=win64&lang=en-US
              windows/arm64: https://download.mozilla.org/?product=firefox-latest&os=win64-aarch64&lang=en-US
            to: ./firefox-update.exe
            title: Downloading Firefox update
            timeout: 15m
            exit_on_failure: true

          - type: run
            command: '.\firefox-update.exe /S'
            title: Installing Firefox update silently
            timeout: 10m
            exit_on_failure: true

      clear-windows-registry:
        available_in: [ready]
        steps:
          - type: run
            command: 'reg delete "HKCU\Software\Mozilla\Firefox" /f'
            title: Clearing Firefox registry entries
            timeout: 30s
            exit_on_failure: false

  "darwin/*":
    requirements:
      cpu_cores: 2
      ram_gb: 2
      disk_gb: 3

    lifecycle:
      install:
        - type: fetch
          url:
            darwin/amd64: https://download.mozilla.org/?product=firefox-130.0&os=osx&lang=en-US
            darwin/arm64: https://download.mozilla.org/?product=firefox-130.0&os=osx-aarch64&lang=en-US
          to: ./Firefox.dmg
          title: Downloading Firefox disk image
          timeout: 15m
          exit_on_failure: true

        - type: run
          command: hdiutil attach ./Firefox.dmg -mountpoint /Volumes/Firefox -nobrowse -quiet
          title: Mounting Firefox disk image
          timeout: 2m
          exit_on_failure: true

        - type: run
          command: cp -R /Volumes/Firefox/Firefox.app ${INSTALL_PATH}/Firefox.app
          title: Copying Firefox to install directory
          timeout: 3m
          exit_on_failure: true

        - type: run
          command: hdiutil detach /Volumes/Firefox -quiet
          title: Unmounting disk image
          timeout: 1m
          exit_on_failure: false

        - type: run
          command: ${INSTALL_PATH}/Firefox.app/Contents/MacOS/firefox --createprofile ${FIREFOX_PROFILE}
          title: Creating Firefox profile
          timeout: 1m
          exit_on_failure: true

      execute:
        - type: run
          command: open -a ${INSTALL_PATH}/Firefox.app --args --profile ${FIREFOX_PROFILE}
          title: Launching Firefox
          timeout: 15s
          exit_on_failure: true

      stop:
        - type: run
          command: osascript -e 'quit app "Firefox"'
          title: Stopping Firefox
          timeout: 10s
          exit_on_failure: false

      uninstall:
        - type: run
          command: rm -rf ${INSTALL_PATH}/Firefox.app ./Firefox.dmg
          title: Removing Firefox
          timeout: 2m
          exit_on_failure: false

    methods:
      update:
        available_in: [ready]
        steps:
          - type: fetch
            url:
              darwin/amd64: https://download.mozilla.org/?product=firefox-latest&os=osx&lang=en-US
              darwin/arm64: https://download.mozilla.org/?product=firefox-latest&os=osx-aarch64&lang=en-US
            to: ./Firefox-update.dmg
            title: Downloading Firefox update disk image
            timeout: 15m
            exit_on_failure: true

          - type: run
            command: hdiutil attach ./Firefox-update.dmg -mountpoint /Volumes/FirefoxUpdate -nobrowse -quiet
            title: Mounting update disk image
            timeout: 2m
            exit_on_failure: true

          - type: run
            command: cp -R /Volumes/FirefoxUpdate/Firefox.app ${INSTALL_PATH}/Firefox.app
            title: Applying Firefox update
            timeout: 3m
            exit_on_failure: true

          - type: run
            command: hdiutil detach /Volumes/FirefoxUpdate -quiet
            title: Unmounting update disk image
            timeout: 1m
            exit_on_failure: false

      set-default-browser:
        available_in: [ready]
        steps:
          - type: run
            command: defaultbrowser firefox
            title: Setting Firefox as default browser (requires defaultbrowser CLI)
            timeout: 30s
            exit_on_failure: false
```

---

### 11.4 Cross-platform binary, shared recipe — Go/Rust CLI with `_common` + `base:`

**Tier 2.** A Go or Rust CLI tool compiled for all six platforms. The execute/stop/uninstall
and all methods are identical across OS families; only `install` differs (download URL, binary
name, `chmod` on Unix). An abstract `_common` base holds the shared structure; each OS provides
only its `install` steps. Overrideable `command` fields use `"windows/*"` + `default` glob keys
to handle the `./mytool` vs `.\mytool.exe` binary name difference.

```yaml
schema: "arrow@v0"

metadata:
  name: char2cs.mytool
  description: My cross-platform CLI tool
  version: 2.1.0
  license: MIT
  quiver: github.com/char2cs/tools.quiver
  maintainers:
    - name: char2cs
      email: me@char2cs.net
  tags:
    - cli
    - developer-tools

variables:
  - name: LISTEN_ADDR
    type: string
    default: "0.0.0.0:8080"
    description: Address and port the server binds to
    sensitive: false

targets:
  # Abstract base — shared execute/stop/methods across all platforms
  _common:
    lifecycle:
      execute:
        - type: run
          command:
            default: ./mytool serve --addr ${LISTEN_ADDR}
            "windows/*": '.\mytool.exe serve --addr ${LISTEN_ADDR}'
          title: Starting mytool server
          timeout: 10s
          exit_on_failure: true

      stop:
        - type: signal
          signal: graceful
          timeout: 10s
          exit_on_failure: false

      uninstall: []

    methods:
      version:
        available_in: [ready, running]
        steps:
          - type: run
            command:
              default: ./mytool --version
              "windows/*": '.\mytool.exe --version'
            title: Checking installed version
            timeout: 5s
            exit_on_failure: true

      config-reset:
        available_in: [ready]
        steps:
          - type: run
            command:
              default: ./mytool config reset
              "windows/*": '.\mytool.exe config reset'
            title: Resetting configuration to defaults
            timeout: 10s
            exit_on_failure: true

  # Concrete targets — only install differs

  "linux/*":
    base: _common
    requirements:
      cpu_cores: 1
      ram_gb: 1
      disk_gb: 1

    lifecycle:
      install:
        - type: fetch
          url:
            linux/amd64: https://github.com/char2cs/mytool/releases/download/v2.1.0/mytool-linux-amd64
            linux/arm64: https://github.com/char2cs/mytool/releases/download/v2.1.0/mytool-linux-arm64
          to: ./mytool
          title: Downloading mytool
          timeout: 5m
          exit_on_failure: true

        - type: run
          command: chmod +x ./mytool
          title: Setting executable bit
          timeout: 10s
          exit_on_failure: true

  "darwin/*":
    base: _common
    requirements:
      cpu_cores: 1
      ram_gb: 1
      disk_gb: 1

    lifecycle:
      install:
        - type: fetch
          url:
            darwin/amd64: https://github.com/char2cs/mytool/releases/download/v2.1.0/mytool-darwin-amd64
            darwin/arm64: https://github.com/char2cs/mytool/releases/download/v2.1.0/mytool-darwin-arm64
          to: ./mytool
          title: Downloading mytool
          timeout: 5m
          exit_on_failure: true

        - type: run
          command: chmod +x ./mytool
          title: Setting executable bit
          timeout: 10s
          exit_on_failure: true

  "windows/*":
    base: _common
    requirements:
      cpu_cores: 1
      ram_gb: 1
      disk_gb: 1

    lifecycle:
      install:
        - type: fetch
          url:
            windows/amd64: https://github.com/char2cs/mytool/releases/download/v2.1.0/mytool-windows-amd64.exe
            windows/arm64: https://github.com/char2cs/mytool/releases/download/v2.1.0/mytool-windows-arm64.exe
          to: ./mytool.exe
          title: Downloading mytool
          timeout: 5m
          exit_on_failure: true
      # no chmod step — Windows executables need no permission change
```

**What `base:` contributes here:** the three concrete targets each inherit
`lifecycle.execute`, `lifecycle.stop`, `lifecycle.uninstall`, and both methods from `_common`.
Each adds only `lifecycle.install` — which it owns entirely; `_common` has no `install` hook
to conflict with. The Overrideable `command` fields in `_common` use `"windows/*"` + `default`
glob keys — valid because the concrete targets collectively cover all of `windows/*`,
`linux/*`, and `darwin/*`, and `default` handles the Unix cases.

---

## 12. Honest gaps

The following are explicit non-goals for `arrow@v0`:

1. **Distro-level variance.** Ubuntu vs. Alpine vs. Arch Linux cannot be distinguished by
   target keys. Use runtime detection in shell commands.

2. **Libc variant targeting.** glibc vs. musl is not addressable by target keys. Ship a
   statically-linked binary or detect at install time in a `run` step.

3. **OS version gating.** Windows 10 vs. Windows 11, macOS Sequoia vs. Ventura — not
   expressible as target keys. Handle in step commands.

4. **Non-primary OS support.** The `GOOS/GOARCH` enum covers Linux, Windows, and Darwin only.
   BSDs, illumos, and others are out of scope for v0 but trivially addable by extending
   `internal/domain/os.go`.

5. **Sub-method step-level override.** A child rewriting a method's step list overrides it
   wholesale. There is no way to override a single step inside a method inherited via `base:`.

6. **Partial install rollback.** If install fails midway, Quiver transitions to `absent` and
   removes the workdir. Steps that produced external side effects (registering a Windows
   service, writing to system directories outside `INSTALL_PATH`) are not rolled back. Manifest
   authors should prefer reversible steps and defer irreversible ones to the end of the install
   sequence.

---

## 13. Migration note

`arrow@v0` is still in active development. The pre-refactor v0 shape — with top-level
`lifecycle:`, `methods:`, `requirements:`, `dependencies:`, and `Overrideable` fields using
bare OS keys — is structurally incompatible with this spec.

The Manifold Translator for `arrow@v0` **must reject** manifests that lack a `targets:` section
with a clear error:

> `This manifest uses the pre-refactor arrow@v0 shape (no "targets:" section). Rewrite it
> according to docs/spec/arrow/v0/manifest.md. No migration shim is provided — v0 is still
> in development.`

Similarly, Overrideable keys in the old bare-OS format (`linux`, `windows`, `macos`) must be
rejected per validation rule 7.
