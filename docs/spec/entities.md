# Quiver — Core Entities

## Overview

Quiver is a universal, decentralized package manager. It simplifies software installation for end users — game servers, utilities, developer tools, and more — while giving developers a free, open platform to publish and distribute their software.

Three foundational concepts form the backbone of Quiver:

1. **Arrows** — Packages. The actual software you install and run.
2. **Quivers** — Stores. Curated catalogs that help users discover Arrows.
3. **Namespaces** — Identity. URL-based, globally unique identifiers for every Arrow and Quiver.

---

## 1. Arrows (Packages)

An Arrow is a piece of software that can be installed, configured, and managed through Quiver. It is defined by a YAML manifest that describes what it is, what it needs, and how to manage its lifecycle.

### What an Arrow contains

| Section | Purpose |
|---------|---------|
| Metadata | Name, description, version, license, maintainers, tags |
| Requirements | Minimum system resources (CPU, RAM, disk) and supported OS |
| Dependencies | Other Arrows this one depends on (by full namespace) |
| Variables | User-configurable parameters with types, defaults, and constraints |
| Netbridge | Network port requirements (TCP/UDP) |
| Lifecycle | Platform-managed state transitions (install, execute, stop, uninstall) |
| Methods | Developer-defined custom actions, gated by lifecycle state |

### Where an Arrow lives

An Arrow can exist in two forms:

**Standalone Arrow** — A git repository that IS the arrow. Contains `arrow.yaml` at root.

```
github.com/valve/steamcmd          # The repo
└── arrow.yaml                     # The Arrow manifest
```

**Arrow inside a Quiver** — A YAML file inside a Quiver repository.

```
github.com/char2cs/gaming.quiver   # The Quiver repo
├── quiver.yaml                    # Quiver manifest
├── cs2.yaml                       # Arrow manifest (AUID: cs2)
├── minecraft.yaml                 # Arrow manifest (AUID: minecraft)
└── steamcmd.yaml                  # Arrow manifest (AUID: steamcmd)
```

Both forms are equally valid. A standalone Arrow and an Arrow inside a Quiver have identical manifest formats — only the file naming differs (`arrow.yaml` vs `{auid}.yaml`).

### Arrow lifecycle

The platform enforces two lifecycle hooks: `install` and `uninstall`. Everything else — `execute`, `stop`, custom methods — is up to the developer. The lifecycle adapts to what the Arrow defines:

```
All Arrows:       absent → ready → removed

If execute/stop:  absent → ready → running → stopped → removed
                                    ↑          ↓
                                    └──────────┘
```

### Arrow manifest format

```yaml
schema: "arrow@v0"

# --- Metadata ---
name: "Counter-Strike 2 Dedicated Server"
description: "A basic CS2 SRCDS dedicated server"
version: "0.0.1"
license: "MIT"
url: "https://developer.valvesoftware.com/wiki/Counter-Strike_2"
maintainers:
  - "char2cs"
credits:
  - "Valve Software"
tags:
  - "game-server"
  - "valve"
  - "fps"

# --- System requirements ---
requirements:
  cpu_cores: 2
  memory_gb: 4
  disk_gb: 30
  os:
    - linux
    - windows

# --- Dependencies (always full namespaces) ---
dependencies:
  - github.com/valve/steamcmd

# --- User-configurable variables ---
variables:
  - name: SERVER_HOSTNAME
    type: string
    default: "CS2 Server hosted with Quiver"
    description: "Server display name"

  - name: MAX_PLAYERS
    type: number
    default: 12
    min: 2
    max: 64
    description: "Maximum concurrent players"

  - name: SERVER_PASSWORD
    type: string
    default: ""
    sensitive: true
    description: "Server access password"

  - name: DEFAULT_MAP
    type: select
    default: "de_dust2"
    values: ["de_dust2", "de_mirage", "de_inferno", "de_anubis"]
    description: "Default map to load"

# --- Network port requirements ---
netbridge:
  - name: GAME_PORT
    protocol: tcp/udp
    default: 27015
    required: true

  - name: RCON_PORT
    protocol: tcp
    default: 27015
    required: false

# --- Lifecycle: platform-managed state transitions ---
#
#   install:    absent  → ready
#   execute:    ready   → running    (optional — omit for static packages)
#   stop:       running → stopped    (required when execute is defined)
#   uninstall:  *       → removed    (optional — platform cleans workdir by default)

lifecycle:
  install:
    - type: run
      command: "${github.com/valve/steamcmd.execute} +login anonymous +force_install_dir ${INSTALL_PATH} +app_update 730 validate +quit"
      title: "Installing CS2 via SteamCMD"
      timeout: 30m

    - type: run
      command: "${INSTALL_PATH}/setup_config.sh --hostname ${SERVER_HOSTNAME} --map ${DEFAULT_MAP} --maxplayers ${MAX_PLAYERS}"
      title: "Configuring server"

  execute:
    - type: run
      command: "${INSTALL_PATH}/cs2 -dedicated -console +hostname ${SERVER_HOSTNAME} +map ${DEFAULT_MAP} +maxplayers ${MAX_PLAYERS} -port ${GAME_PORT}"
      title: "Starting CS2 server"

  stop:
    - type: signal
      signal: SIGTERM
      timeout: 30s

  uninstall:
    - type: run
      command: "${INSTALL_PATH}/cleanup.sh"
      title: "Cleaning up server files"

# --- Methods: developer-defined custom actions ---
#
#   available_in: which lifecycle states this method can be invoked in
#   steps:        sequential list of actions to execute

methods:
  update:
    available_in: [ready, stopped]
    steps:
      - type: run
        command: "${github.com/valve/steamcmd.execute} +login anonymous +force_install_dir ${INSTALL_PATH} +app_update 730 validate +quit"
        title: "Updating CS2"
        timeout: 30m

  validate:
    available_in: [ready, stopped]
    steps:
      - type: run
        command: "test -f ${INSTALL_PATH}/cs2"
        title: "Validating game files"

  change-map:
    available_in: [running]
    steps:
      - type: run
        command: "${INSTALL_PATH}/rcon.sh changelevel ${DEFAULT_MAP}"
        title: "Changing map"

  backup:
    available_in: [ready, stopped]
    steps:
      - type: run
        command: "${INSTALL_PATH}/backup.sh --output ${DATA_PATH}/backups/"
        title: "Backing up server data"
```

### Manifest field reference

#### `schema` (required)
Manifest format version. Format: `arrow@v{version}`. Allows Quiver.core to reject or upcast manifests with unknown versions.

#### `name` (required)
Human-readable name for the Arrow.

#### `description` (required)
Short description of what this Arrow does.

#### `version` (required)
The software version this Arrow installs (semver recommended).

#### `license` (optional)
SPDX license identifier.

#### `url` (optional)
Link to the software's homepage or documentation.

#### `maintainers` (required)
List of people maintaining this Arrow manifest.

#### `credits` (optional)
Attribution to original software authors.

#### `tags` (optional)
String array for discoverability in the store UI. No rigid taxonomy — free-form.

#### `requirements` (required)
Minimum system resources:
- `cpu_cores` — Minimum CPU cores (integer)
- `memory_gb` — Minimum RAM in GB (integer)
- `disk_gb` — Minimum disk space in GB (integer)
- `os` — List of supported operating systems: `linux`, `windows`, `macos`

#### `dependencies` (optional)
List of Arrow namespaces this Arrow depends on. **Must use full namespaces** — never bare AUIDs. Examples:
- `github.com/valve/steamcmd` (standalone Arrow)
- `github.com/char2cs/gaming.quiver/steamcmd` (Arrow inside a Quiver)

Quiver.core resolves and installs dependencies before the Arrow itself.

#### `variables` (optional)
User-configurable parameters. Each variable has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Variable identifier (used in interpolation) |
| `type` | string | yes | One of: `string`, `number`, `boolean`, `select` |
| `default` | any | yes | Default value |
| `description` | string | no | Human-readable explanation |
| `sensitive` | boolean | no | If true, value is masked in UI/logs (default: false) |
| `min` | number | no | Minimum value (for `number` type) |
| `max` | number | no | Maximum value (for `number` type) |
| `values` | string[] | no | Allowed values (required for `select` type) |

#### `netbridge` (optional)
Network port requirements. Each entry has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Port identifier (used in interpolation) |
| `protocol` | string | yes | One of: `tcp`, `udp`, `tcp/udp` |
| `default` | integer | yes | Default port number |
| `required` | boolean | no | Whether this port is mandatory (default: true) |

#### `lifecycle` (required)
Platform-managed state transitions. The platform owns the state machine; the Arrow provides the implementation for each transition.

| Hook | Required | Transition | Description |
|------|----------|------------|-------------|
| `install` | yes | `absent → ready` | Provision and set up the software |
| `execute` | no | `ready → running` | Start a long-running process |
| `stop` | if `execute` exists | `running → stopped` | Terminate the running process |
| `uninstall` | no | `* → removed` | Clean up before removal (platform cleans workdir by default) |

Each hook contains a sequential list of steps. Every step has a `type` field that identifies which kind of action it is:

| `type` | Description | Required fields |
|--------|-------------|-----------------|
| `run` | Execute a shell command | `command` |
| `fetch` | Download a remote file | `url`, `to` |
| `signal` | Send a process signal | `signal` |

Step options:
- `title` — Human-readable description shown in UI
- `timeout` — Maximum execution time (e.g., `30m`, `5s`)
- `exit_on_failure` — Whether to abort on failure (default: `true`)

#### `methods` (optional)
Developer-defined custom actions. Unlike lifecycle hooks, methods do not transition the Arrow between states — they are actions the user can invoke when the Arrow is in a specific state.

Each method has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `available_in` | string[] | yes | Lifecycle states where this method can be invoked |
| `steps` | list | yes | Sequential list of steps (same format as lifecycle steps) |

Valid states for `available_in`: `ready`, `running`, `stopped`.

### Variable interpolation

Lifecycle and method steps support variable interpolation with `${}` syntax:

| Syntax | Resolves to | Example |
|--------|-------------|---------|
| `${VAR_NAME}` | A variable defined in `variables:` | `${SERVER_HOSTNAME}` |
| `${PORTNAME}` | A port defined in `netbridge:` | `${GAME_PORT}` |
| `${namespace.action}` | A dependency's action output | `${github.com/valve/steamcmd.execute}` |

**Built-in variables** (managed by Quiver.core, always available):

| Variable | Description |
|----------|-------------|
| `${INSTALL_PATH}` | Directory where this Arrow's files are installed |
| `${DATA_PATH}` | Directory for persistent data (survives updates) |
| `${QUIVER_HOME}` | Quiver's root directory |
| `${ARROW_NAMESPACE}` | This Arrow's full namespace |
| `${PLATFORM}` | Current platform (`linux`, `windows`, `macos`) |

---

## 2. Quivers (Stores / Catalogs)

A Quiver is a curated catalog of Arrows. It helps users discover software through the store interface. It does NOT own Arrows — it references them. Think of it like a playlist: songs (Arrows) exist independently, playlists (Quivers) curate them.

### What a Quiver does

- **Catalogs Arrows** — Lists available software for users to browse
- **Aids discovery** — Populates the store UI when a user subscribes
- **Signals trust** — The official Rabbyte.quiver marks certain Quivers as trusted

### What a Quiver does NOT do

- Does not "own" or "contain" Arrows in a logical sense
- Is not required to install an Arrow (direct install by namespace works)
- Does not create a namespace boundary for external Arrows

### Where a Quiver lives

A Quiver is a git repository with `quiver.yaml` at root:

```
github.com/char2cs/gaming.quiver
├── quiver.yaml            # Quiver manifest (required)
├── cs2.yaml               # Local Arrow (optional)
├── minecraft.yaml         # Local Arrow (optional)
└── steamcmd.yaml          # Local Arrow (optional)
```

Arrow YAML files sit alongside `quiver.yaml` in the repo root. Flat structure — no subdirectories.

### Quiver manifest format

```yaml
schema: "quiver@v0"

# --- Metadata ---
name: "Gaming Quiver"
description: "Game servers and utilities curated by char2cs"
url: "https://gaming.quiver.ar"
maintainers:
  - "char2cs"
tags:
  - "gaming"
  - "servers"

media:
  icon: "https://example.com/icon.png"
  banner: "https://example.com/banner.png"

# --- Arrow catalog ---
arrows:
  # Local arrows (files in this repo)
  - cs2
  - minecraft

  # External arrows (pointers — not re-namespaced)
  - github.com/valve/steamcmd
  - github.com/valve/quiver/steamcmd
```

### Manifest field reference

#### `schema` (required)
Manifest format version. Format: `quiver@v{version}`.

#### `name` (required)
Human-readable name for this store.

#### `description` (required)
What kind of software this Quiver catalogs.

#### `url` (optional)
Link to the Quiver's homepage or documentation.

#### `maintainers` (required)
List of people maintaining this Quiver.

#### `tags` (optional)
String array for discoverability. Free-form.

#### `media` (optional)
Store UI assets:
- `icon` — Small image for listings
- `banner` — Large image for the store page

#### `arrows` (required)
List of Arrows available in this store. Two forms:

| Form | Example | Meaning |
|------|---------|---------|
| Simple name | `cs2` | Local Arrow file in this repo (`cs2.yaml`). Namespace becomes `{this-quiver-url}/cs2` |
| Full namespace | `github.com/valve/steamcmd` | External Arrow. Keeps its own namespace. Quiver is just a pointer. |

### How Quivers reference Arrows

When a Quiver lists a **local** arrow (simple name like `cs2`), Quiver.core:
1. Looks for `cs2.yaml` in the Quiver repo
2. Assigns it the namespace `github.com/char2cs/gaming.quiver/cs2`

When a Quiver lists an **external** arrow (full URL like `github.com/valve/steamcmd`), Quiver.core:
1. Resolves the URL to the external repo
2. The Arrow keeps its own namespace — it is NOT re-namespaced under the Quiver

This means the same Arrow can appear in many Quivers without identity conflicts.

---

## 3. Namespaces (Identity)

Every Arrow and Quiver in the Quiver ecosystem has a globally unique namespace. Namespaces are **URL-based**, following the same philosophy as Go modules: the identifier IS the location.

### Why URL-based namespaces

| Problem | How URLs solve it |
|---------|-------------------|
| Name collisions | Impossible — URLs are globally unique by domain ownership |
| Central registry needed | Not needed — anyone with a git repo can publish |
| Ambiguous dependencies | Full URL in every dependency reference, always unambiguous |
| Platform lock-in | Works with any git platform (GitHub, GitLab, Gitea, self-hosted) |

### The two namespace forms

There are exactly two forms of namespace in Quiver:

#### Form 1: Standalone (`domain/user/repo`)

Identifies a git repository that is either a standalone Arrow or a Quiver.

```
github.com/valve/steamcmd              # Standalone Arrow
github.com/char2cs/gaming.quiver       # Quiver
gitlab.com/company/internal-tools      # Either — determined by manifest
```

**Resolution:** Quiver.core fetches the repo and checks the root:
- Has `arrow.yaml` → it's a standalone Arrow
- Has `quiver.yaml` → it's a Quiver
- Has neither → error
- Has both → error (a repo must be one or the other)

#### Form 2: Arrow inside a Quiver (`domain/user/repo/auid`)

Identifies a specific Arrow file within a Quiver repository. The fourth path segment is the AUID.

```
github.com/char2cs/gaming.quiver/cs2         # cs2.yaml in the Quiver repo
github.com/char2cs/gaming.quiver/minecraft   # minecraft.yaml in the Quiver repo
```

**Resolution:** Quiver.core fetches the Quiver repo, then looks for `{auid}.yaml`.

### The fourth segment rule

Standalone namespaces always have exactly three segments (`domain/user/repo`). When a fourth segment is present (`domain/user/repo/auid`), it identifies an Arrow inside a Quiver. The fourth segment is **always a simple identifier** — never a URL or nested path.

```
github.com/char2cs/gaming.quiver/cs2                              # VALID — 4 segments
github.com/char2cs/gaming.quiver/steamcmd                         # VALID — 4 segments
github.com/char2cs/gaming.quiver/github.com/valve/steamcmd        # INVALID — AUID must be simple
```

### AUID format

The Arrow Unique ID (AUID) is the fourth segment of a Quiver-hosted namespace. It is also the filename (without `.yaml` extension) inside a Quiver repo.

**Constraints:**
- Lowercase alphanumeric characters and hyphens only: `[a-z0-9\-]+`
- Must start with a letter: `[a-z]`
- Maximum 64 characters
- Examples: `cs2`, `steamcmd`, `minecraft-vanilla`, `obs-studio`

### Namespace resolution

Quiver.core resolves namespaces to fetchable locations:

```
Namespace                                    Git repo URL
─────────────────────────────────────────    ──────────────────────────────────────
github.com/valve/steamcmd                    https://github.com/valve/steamcmd
github.com/char2cs/gaming.quiver             https://github.com/char2cs/gaming.quiver
github.com/char2cs/gaming.quiver/cs2         https://github.com/char2cs/gaming.quiver (then find cs2.yaml)
gitlab.com/company/tools                     https://gitlab.com/company/tools
```

**Known platforms** (github.com, gitlab.com, bitbucket.org): Quiver.core derives the git clone URL directly from the namespace.

**Custom domains** (future): HTTP meta-tag discovery, similar to Go's `?go-get=1` mechanism. A request to `https://custom.domain/my-arrow` would return a meta tag pointing to the actual git repo.

### How dependencies use namespaces

Dependencies in an Arrow manifest always use **full namespaces**:

```yaml
# In github.com/char2cs/gaming.quiver/cs2
dependencies:
  - github.com/valve/steamcmd                         # Standalone Arrow
  - github.com/char2cs/gaming.quiver/steamcmd          # Arrow in same Quiver
  - github.com/other/tools.quiver/7zip                 # Arrow in different Quiver
```

There is no shorthand. No bare names. This eliminates all ambiguity — every dependency points to exactly one Arrow, regardless of how many Quivers the user has added.

### Edge cases

**Same AUID in different Quivers:**
- `github.com/alice/quiver/steamcmd`
- `github.com/bob/quiver/steamcmd`

Different namespaces. No collision. Both can coexist.

**Standalone Arrow with same name as Quiver-hosted Arrow:**
- `github.com/valve/steamcmd` (standalone)
- `github.com/char2cs/gaming.quiver/steamcmd` (in Quiver)

Different namespaces. Both can be installed simultaneously.

**Quiver lists both a local and external Arrow with same AUID:**
```yaml
arrows:
  - steamcmd                        # github.com/char2cs/gaming.quiver/steamcmd
  - github.com/valve/steamcmd       # github.com/valve/steamcmd
```

Valid. They are different Arrows with different namespaces. The store UI shows both.

**Circular Quiver references:**
- Quiver A references Arrows from Quiver B
- Quiver B references Arrows from Quiver A

Not a problem. Quivers are catalogs — they don't create dependency chains. Only Arrow-to-Arrow dependencies can be circular, and Quiver.core rejects those at install time.

**Adding a standalone Arrow as a Quiver (or vice versa):**
```
quiver add-store github.com/valve/steamcmd
→ Error: "This is an Arrow, not a Quiver. Use 'quiver install github.com/valve/steamcmd' instead."

quiver install github.com/char2cs/gaming.quiver
→ Error: "This is a Quiver, not an Arrow. Use 'quiver add-store' instead."
```

---

## 4. How They Relate

```
                    ┌─────────────────────────────────────────────────────┐
                    │              User's Quiver Instance                 │
                    │                                                     │
                    │  Subscribed Quivers (Stores):                       │
                    │  ┌───────────────────────────────────────────┐      │
                    │  │ github.com/char2cs/gaming.quiver          │      │
                    │  │   arrows: [cs2, minecraft, steamcmd,      │      │
                    │  │            github.com/valve/steamcmd]     │      │
                    │  └───────────────────────────────────────────┘      │
                    │                                                     │
                    │  Installed Arrows:                                  │
                    │  ┌───────────────────────────────────────────┐      │
                    │  │ github.com/char2cs/gaming.quiver/cs2      │      │
                    │  │   state: running                          │      │
                    │  │ github.com/valve/steamcmd                 │      │
                    │  │   state: ready                            │      │
                    │  └───────────────────────────────────────────┘      │
                    │                                                     │
                    └─────────────────────────────────────────────────────┘
```

### User flow

1. **Add a Quiver (store):**
   ```
   quiver add-store github.com/char2cs/gaming.quiver
   ```
   Quiver.core fetches the manifest and indexes all listed Arrows in the store.

2. **Browse the store:**
   ```
   quiver store list
   ```
   Shows all Arrows from all subscribed Quivers.

3. **Install an Arrow:**
   ```
   quiver install github.com/char2cs/gaming.quiver/cs2
   ```
   Quiver.core resolves the namespace, fetches the manifest, checks requirements, resolves dependencies, runs the `install` lifecycle hook, and transitions the Arrow to `ready`.

4. **Execute a service Arrow:**
   ```
   quiver execute github.com/char2cs/gaming.quiver/cs2
   ```
   Quiver.core runs the `execute` lifecycle hook, transitioning the Arrow from `ready` to `running`.

5. **Run a custom method:**
   ```
   quiver run github.com/char2cs/gaming.quiver/cs2 backup
   ```
   Quiver.core checks the Arrow's current state against the method's `available_in`, then executes the steps.

6. **Direct install (no Quiver needed):**
   ```
   quiver install github.com/valve/steamcmd
   ```
   Works without subscribing to any Quiver. The namespace is enough.
