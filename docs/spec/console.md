# Quiver — Console

## Overview

The console lets a client (quiver.desktop's quake-style console) read the daemon's own logs and run the daemon's own CLI commands. Three routes, all under `/v0`, all behind the same bearer-token gate as the rest of the API:

| Route | Purpose |
|---|---|
| `GET /v0/console/logs` (WebSocket) | Replay of recent log records, then the live stream |
| `GET /v0/console/commands` | The commands the console may run, for help and completion |
| `POST /v0/console/exec` | Run one command line, streamed as NDJSON |

A daemon advertises the feature in `GET /versions` (`features` contains `console.v1`). A client treats a missing `features` field as "none".

Source: `internal/console/logring` (log capture), `internal/console/command` (execution and policy), `internal/api/v0/endpoints/console` (HTTP/WS). The desktop side of the contract is in quiver.desktop's `docs/console-spec.md`; both files describe the same wire format.

## 0. Principles

1. **Default-deny.** Nothing is reachable from the console unless it is explicitly opted in.
2. **No second command list.** The daemon executes its own cobra tree (`internal/cli/commands`). The only policy data is a per-command annotation next to each command's definition (`clierr.AllowInConsole`).
3. **No shell, ever.** The line is tokenised by a small quote-aware lexer into `[]string` and handed to cobra. No `sh -c`, no environment expansion, no globbing, no pipes or redirection, no subshells. `$`, backticks, `;`, `|`, `&`, `<`, `>`, `*`, `?` and `~` are ordinary characters.
4. **Same auth as the rest of the API.** The routes sit behind `AuthGate.RequireBearer()`. `unix://` and `npipe://` are trusted by filesystem permissions, TCP needs a paired device. Commands run with the **caller's own credentials**: the bearer token of the exec request is forwarded to the in-process CLI session, so a console command can do nothing the caller could not already do with the REST API.
5. **The daemon never blocks on a console client.** Slow log consumers are dropped, never waited on.
6. **Structured, not parsed.** Logs are captured as `slog.Record` values, not parsed from stdout, so they do not depend on whether the file logger is enabled or whether the handler writes JSON or text.

## 1. Build identity and capability

`GET /versions` carries, in `data`:

```json
{
  "version": "...", "build_id": "...",
  "commit": "9dd0b183177a64ec71a2672d1cd7cf0c70bb4877",
  "built_at": "2026-10-04T13:47:00Z",
  "channel": "nightly-latest",
  "features": ["console.v1"],
  "api": { "supported": ["v0"], "latest": "v0" }
}
```

- `commit`, `built_at` and `channel` are stamped by ldflags (`main.commit`, `main.builtAt`, `main.channel`) in the Makefile, the Dockerfile, `build-assets.yml` and the e2e `selfbuild.sh`. They are empty strings for an unstamped dev build.
- `build_id` is days elapsed since the Quiver epoch (unchanged).
- `channel` is whatever the release pipeline passes: `stable`, `beta`, `hotfix` or `nightly-latest` (the selector the nightly build is published under).
- `features` is additive.

## 2. `GET /v0/console/logs` (WebSocket)

Query:

| Param | Meaning |
|---|---|
| `level` | `debug`, `info`, `warn` or `error` (default `info`). Server-side minimum level. Anything else is a 400. |
| `since` | Unsigned sequence number. Replay only records with `seq > since` still in the ring. |
| `replay` | Most records to replay (default 500, at most 2000). `0` skips the replay. |

Behaviour:

- A process-wide ring (5000 records) is fed by a `slog.Handler` tee installed by `logger.Init`. The tee wraps whatever handler is configured and never changes where logs go. It captures at the level the configured handler emits: a daemon configured for `info` has no `debug` records to show.
- Each record gets a `seq`: a `uint64`, starting at 1, per daemon process.
- Order on the wire: replay frames, **one** `ready` frame, then live frames. The subscriber is registered before the replay snapshot is taken and live frames at or below the last replayed `seq` are skipped, so a record is neither lost nor repeated across the seam.
- `since` greater than the newest `seq` means the daemon restarted (sequences are per process). The cursor is discarded, the last `replay` records are sent, and `ready` carries `"reset": true` so the client can clear what it holds.
- **Slow consumers.** Each subscriber has a bounded queue (256). On overflow the record is dropped and counted; the next frame sent to that client is preceded by a `gap` frame with the count. A queue that stays full for 5 seconds closes the connection with code 1013. Writes carry a 5 second deadline. Pings every 30 seconds, pong timeout 60 seconds, client frames are limited to 1 KiB and ignored.
- **Redaction** is applied before a record is stored, so replay and live frames are identical:
  - any attribute whose key matches `(?i)(token|secret|password|passwd|authorization|api[-_]?key|private[-_]?key|bearer|cookie|credential|pairing)` has its value replaced by `[redacted]`;
  - inside any string value, `Bearer <credential>` and URL userinfo (`scheme://user:pass@`) are replaced;
  - values longer than 2 KiB are truncated with a `…` suffix on a rune boundary; messages too;
  - at most 32 attributes per record, extra ones dropped and `fields_truncated: true` set; nested groups are flattened with dotted keys.

Frames (JSON text frames):

```json
{"type":"log","seq":412,"time":"2026-10-04T14:02:14.390123Z","level":"warn","component":"release","msg":"channel lookup slow","fields":{"ns":"github.com/char2cs/crowbar","took":"1.8s","retry":1},"fields_truncated":false}
{"type":"ready","seq":412}
{"type":"ready","seq":3,"reset":true}
{"type":"gap","dropped":37}
```

- `component` is the `component` attribute when present (and then it is not repeated in `fields`), else `""`.
- `fields` values keep their JSON type. Durations are Go `time.Duration.String()` strings, errors are their `Error()` text, times are RFC 3339.
- `ready.seq` is the sequence of the last frame sent so far (or the discarded-then-zero cursor), so a client reconnects with `since=<last received seq>`.

## 3. `GET /v0/console/commands`

Returns the opted-in command tree, so clients render help and completion without a list of their own:

```json
{"success":true,"data":{"commands":[
  {"path":["arrow","add"],"short":"Register an arrow in the catalog","usage":"arrow add <namespace>","aliases":[],
   "flags":[{"name":"yes","shorthand":"y","usage":"skip the confirmation prompt","takes_value":false}]}
]}}
```

Only commands that carry the console annotation, whose ancestors do too, that are runnable and not hidden appear. Flags the console refuses (see § 4) are omitted. `flags` is additive to the original contract.

## 4. `POST /v0/console/exec`

Request: `{"line":"arrow add github.com/char2cs/crowbar"}` (no leading `quiver`). Unknown JSON fields and bodies over 4 KiB are a 400.

Response: `200`, `application/x-ndjson`, one frame per line, flushed as produced:

```json
{"type":"out","stream":"stdout","data":"resolving …\n"}
{"type":"out","stream":"stderr","data":"warning: …\n"}
{"type":"exit","code":0,"error":""}
```

- Exactly one `exit` frame ends every response that got past validation. `code` follows the CLI's exit codes (0 ok, 1 failure, 2 usage, 3 daemon unreachable, 130 interrupted). `error` is the message the CLI would print after `quiver:`.
- Failures before execution are ordinary JSON error envelopes: `400` (empty, oversized, unparseable line, bad body), `403` (command not available in the console, the message names it), `401` (auth), `429` (concurrency cap), `503` (the daemon's own address is not known yet), `500`.
- Limits: line ≤ 1024 bytes and ≤ 64 arguments; output capped at 256 KiB per call (past it a single notice frame is sent, the rest is discarded, **the command keeps running and its exit code is unaffected**); wall-clock timeout 10 minutes (`timed out after 10m0s` on the exit frame); at most 2 concurrent commands per device (`local` for unix) and 8 overall. A denied command does not hold a slot.
- The request context cancels the command when the client disconnects.

### Execution rules

- Each call builds a **fresh** command tree (`commands.New(deps).Attach(root)`): flags set by one call can never leak into the next.
- The tree is probed first on a throwaway copy (`command.AuthorizeFlags`: resolve, check, parse flags), then built again to run. A root `PersistentPreRunE` re-checks, at execution time, that cobra is about to run exactly the command that was authorized and that no denied flag was set.
- Reachability (`command.Authorize`): the resolved command and **every ancestor** must carry `quiver_console=true`, it must be runnable and not hidden. The root itself is never reachable: its handler dispatches `<namespace> [method]` to arbitrary manifest methods, which the console does not expose. Cobra's prefix matching, case-insensitive matching and traverse-run-hooks switches are asserted off by tests; `help`, `completion` and `__complete` are never reachable.
- Refused in any spelling before cobra runs: `--server`, `--context`, `--config` (and `=value` forms). They redirect a command to another server or another config file. After parsing, a `Changed` check repeats this and also refuses per-command denied flags (`status --watch`/`-w`).
- The session is injected (`commands.Deps.Session`): it dials the daemon's own address (recorded by `internal/internal.go` once the listener is bound, `gateway.DialURI`; a TCP listener on `0.0.0.0` is reached through loopback) with the caller's own bearer token, never reads or writes the CLI configuration (`Config()` fails), never starts a daemon, and is non-TTY.
- **Stdin is empty.** Confirmation prompts never answer yes; because the session is non-TTY they refuse outright with "requires --yes/-y when not running interactively". `uninstall` and `arrow remove` therefore need `--yes`, typed deliberately.
- Output goes to bounded writers via `SetOut`/`SetErr`; output defaults to `-o table` (plain text; the non-TTY default of `json` is overridden, `-o json|yaml` still works).
- A panic inside a command is recovered into an `exit` frame with code 1 and an error log; the daemon keeps running. A command that ignores its context cannot hold the response past the timeout; its late output is discarded.
- **Audit.** Every call logs one info record `msg="exec"`, `component=console`, with `device` (device id, or `local`), `line` (run through `logring.RedactLine`), `code` and `took`. Denied lines log `exec denied` at warn, also redacted.

### Which commands are reachable

The annotated set (checked exactly by `TestDefaultDeny_TheAnnotatedLeafCommandsAreExactlyTheApprovedSet`; adding a command to the console means editing that test):

| Reachable | Notes |
|---|---|
| `install`, `run`, `stop`, `update`, `uninstall` | Lifecycle verbs. `uninstall` needs `--yes`. |
| `ps`, `status` | `status --watch`/`-w` refused (unbounded stream). |
| `list`, `search`, `info`, `methods` | Read-only discovery. |
| `health`, `version` | |
| `arrow add`, `arrow remove`, `arrow refresh`, `arrow list`, `arrow show` | `arrow remove` needs `--yes`. |
| `collection list`, `collection show` | Read-only. |

Left off on purpose:

| Command | Why |
|---|---|
| `daemon`, `self-update` | Process control. They live outside the annotated tree and the console's tree never contains them. |
| `context …`, `--server`, `--context`, `--config` | Manage or select other servers and the CLI config file. |
| `auth devices …` | Pairing administration is loopback-only by design. |
| `arrow seed`, `collection seed` | `--file` reads an arbitrary path on the daemon's host (or stdin). |
| `path status`, `path setup` | `path setup` edits the user's shell rc files. |
| `watch`, `status --watch` | Unbounded streams. |
| `collection follow`, `unfollow`, `update` | Arguable; desktop has UI for them. Left off for the first release. |
| `<namespace> [method]` (root dispatch) | Arbitrary manifest methods. Not annotated. |
| `help`, `completion`, `__complete` | Cobra built-ins; `<cmd> --help` works on any reachable command. |

`run`, `install` and `update` execute arrow code on the daemon's host. That is exactly what `POST /v0/runtime/{ns}/{method}` already allows any authenticated caller; the console adds no authority.

## 5. Residual risks

- **Secrets in log text.** Redaction is by attribute key and two value patterns. A secret interpolated into a message or an error string under an innocuous key is not recognised. Anyone who can read the console can read the logs.
- **Trusted local socket.** On `unix://` the gate is a no-op, so any local process that can open the socket can run console commands. This is the existing trust model of the whole API, not something the console introduces.
- **No per-command rate limit** beyond the concurrency caps. A caller can still start long commands up to the caps.

## 6. Tests

`internal/console/logring` (ring, wrap, `since`, level, subscription drop and stuck, tee, groups, caps, truncation, redaction, races), `internal/console/command` (tokeniser; default-deny walk over the real tree; ancestor rule; exact approved set; forbidden names; shadowing hooks; cobra switches; denial matrix for aliases, abbreviations, case, help, completion, root dispatch, redirect flags in every spelling, `--watch`; real execution against a fake daemon; config isolation; stdin; no flag leakage between calls; panic and timeout), `cmd/quiver` (the same walk over the real root including `daemon` and `self-update`), `internal/api/v0/endpoints/console` (handlers, limits, caps, timeout, cancel, late output, audit, bearer forwarding, and a stack test with the real auth gate), and `tests/integration/console` (`-tags integration`: the real daemon over its unix socket).
