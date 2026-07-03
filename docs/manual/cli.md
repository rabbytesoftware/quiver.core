# Quiver CLI — User Manual

The `quiver` command is your terminal interface to Quiver. It installs, runs, and manages software published as Arrows, straight from any Git repository.

You never need to start or stop anything by hand — the Quiver daemon boots automatically when a command needs it and shuts down on its own when there is nothing left to manage.

---

## Getting Started

Run `quiver` with no arguments to see the welcome screen, or jump straight in:

```bash
quiver install github.com/valve/steamcmd
```

That's it. Quiver resolves the manifest from the repository, installs any dependencies, and reports progress step by step:

```
  ▸ quiver  install  github.com/valve/steamcmd
    3 of 6  ·  4.2s

    1  ✓  Resolving manifest
    2  ✓  Fetching dependencies
    3  ⣽  Downloading artifacts...
    4  ○  Extracting
    5  ○  [Untitled step]
    6  ○  Configuring runtime
```

When the installation finishes, a summary box confirms the result, the version, and the arrow's state.

### Identifying software: namespaces

Every arrow is identified by where it lives:

```
github.com/valve/steamcmd              latest version
github.com/valve/steamcmd@v1.4.2       pinned to a git tag
github.com/char2cs/gaming.quiver/cs2   arrow inside a collection repository
```

Append `@ref` to pin a version — a tag, a branch, or `latest`.

---

## Everyday Commands

### Install, run, stop

```bash
quiver install github.com/rabbyte/chat        # download and install
quiver run github.com/rabbyte/chat            # start it
quiver stop github.com/rabbyte/chat           # gracefully stop it
quiver update github.com/rabbyte/chat         # update to the latest version
quiver uninstall github.com/rabbyte/chat      # remove it and free resources
```

Useful variations:

```bash
quiver run github.com/rabbyte/chat --detach   # return to the shell immediately
quiver run github.com/rabbyte/chat -- --port 9000   # pass args to the program
quiver stop github.com/rabbyte/chat --force   # skip draining, kill immediately
quiver update --all                           # update everything installed
quiver update --check                         # see what would update, change nothing
quiver install github.com/new/tool -m ./arrow.yaml  # install from a local manifest
```

When you detach, the arrow keeps running in the background:

```
  ◆ github.com/rabbyte/chat is running in background
    daemon will exit automatically when no arrows are active
    run `quiver status github.com/rabbyte/chat` to check
```

### Custom methods

Arrows can ship their own methods beyond the built-in five. Invoke them with the arrow's namespace first:

```bash
quiver github.com/rabbyte/postgres backup
quiver github.com/rabbyte/postgres seed-db --data '{"rows": 1000}'
```

Discover what an arrow offers:

```bash
quiver methods github.com/rabbyte/postgres
```

```
  ▸ quiver  methods  github.com/rabbyte/postgres

  custom      backup · seed-db · migrate
```

Add `--include-builtins` to also list `install`, `uninstall`, `run`, `stop`, and `update`.

Not sure what an arrow can do? `quiver <ns>` alone prints its help panel with state, description, and examples.

---

## Finding Software

```bash
quiver list                        # your installed arrows and followed collections
quiver list --all                  # include the full catalog
quiver search github.com/rabbyte/* # search by namespace pattern
quiver info github.com/rabbyte/chat            # details for one arrow
quiver info github.com/rabbyte/chat --manifest # its full manifest
```

`quiver list` prints your whole vault — arrows first, collections below. Filter either view with a glob:

```bash
quiver list -F github.com/rabbyte/*
```

---

## Monitoring

```bash
quiver ps                                  # what's running right now
quiver ps --all                            # include stopped/ready arrows
quiver status github.com/rabbyte/chat      # current state of one arrow
quiver status github.com/rabbyte/chat -w   # keep watching live
quiver watch github.com/rabbyte/chat       # raw live event stream
```

`quiver status` also shows the outcome of the last operation — if a background install failed while you were away, this is where you find out what went wrong and at which step.

---

## Collections

Collections are curated catalogs of arrows, published as a `collection.yaml` in a Git repository. Follow one to add its arrows to your catalog:

```bash
quiver collection follow github.com/char2cs/gaming.quiver
quiver collection list
quiver collection show github.com/char2cs/gaming.quiver
quiver collection update github.com/char2cs/gaming.quiver   # re-sync from git
quiver collection unfollow github.com/char2cs/gaming.quiver
```

---

## Managing the Catalog

For registering and maintaining arrows without installing them — mostly useful for arrow authors and admins:

```bash
quiver arrow add github.com/new/arrow             # register in the catalog
quiver arrow add github.com/new/arrow -m ./arrow.yaml   # seed from a local file
quiver arrow validate ./arrow.yaml                # check a manifest before publishing
quiver arrow show github.com/new/arrow            # inspect a catalog entry
quiver arrow list                                 # admin view (includes dependencies)
quiver arrow remove github.com/old/arrow          # deregister
quiver arrow remove github.com/old/arrow --cascade  # uninstall first if needed
```

Arrow authors: `quiver arrow validate --strict` fails on warnings too — run it in CI before tagging a release.

---

## Remote Instances

By default the CLI talks to the Quiver on your machine. To manage a Quiver running elsewhere, register it as a context:

```bash
quiver context add homelab --server tcp://192.168.1.10:40257
quiver context use homelab       # all commands now target homelab
quiver context current           # which one am I on?
quiver context list              # all registered instances
quiver context use local         # back to this machine
```

One-off, without switching:

```bash
quiver ps -C homelab
```

Contexts live in `~/.quiver/cli.yaml`.

---

## Scripting

The CLI is pipe-friendly. When output is piped, animations and colors turn off and the format switches to JSON automatically:

```bash
quiver list | jq '.arrows[].namespace'
quiver ps -o json | jq '.[] | select(.state == "running")'
quiver list -o yaml -F github.com/rabbyte/* > backup.yaml
```

Exit codes:

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Operation failed (step failure, timeout) |
| `2` | Usage error (bad command, bad flags) |
| `3` | Cannot reach the daemon or remote instance |

Environment variables mirror the global flags: `QUIVER_CONTEXT`, `QUIVER_HOST`, `QUIVER_OUTPUT`, `QUIVER_TIMEOUT`, `QUIVER_VERBOSE`, `QUIVER_CONFIG`, and `NO_COLOR`.

---

## Global Flags

Available on every command:

| Flag | Short | Description |
|---|---|---|
| `--context <name>` | `-C` | Target a named context for this command only |
| `--server <url>` | `-s` | Target a server directly, bypassing contexts |
| `--output <fmt>` | `-o` | `table` (default on terminal), `json` (default piped), `yaml` |
| `--filter <glob>` | `-F` | Filter list output by namespace pattern |
| `--quiet` | `-q` | Suppress everything except errors |
| `--verbose` | `-v` | Show API request/response detail |
| `--timeout <s>` | `-t` | Cap how long an operation may take |
| `--no-wait` | | Don't wait for completion (same as `--detach`) |
| `--no-color` | | Plain output without ANSI colors |
| `--config <file>` | `-c` | Use an alternate config file |

---

## Troubleshooting

**"cannot reach the daemon" (exit code 3)** — the CLI normally boots the daemon automatically. If it can't, check for a stale socket or PID file under `~/.quiver/` (`quiver.sock`, `quiver.pid`) and remove them if no quiver process is running.

**A background operation seems stuck** — `quiver status <ns>` shows the current step; `quiver watch <ns>` streams events live. Failed operations report the failing step and its error in `status`.

**`quiver daemon` says "already running"** — that's normal; there is only ever one daemon per machine. You generally never need to run `quiver daemon` yourself.

**Checking versions** — `quiver version` prints both the CLI and daemon build; `quiver health` checks that the daemon and its components respond. `quiver health --all-contexts` pings every registered instance.
