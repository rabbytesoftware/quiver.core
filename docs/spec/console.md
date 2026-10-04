# Console

The console lets a client read the daemon's logs and run the daemon's own `quiver` CLI. It is served by `internal/api/v0/endpoints/console`, sits on the same protected route group as the rest of the API (unix socket trusted, TCP needs the bearer token) and is advertised as `"features": ["console.v1"]` on `GET /versions`, next to `commit`, `built_at` and `channel`.

## Logs: `GET /v0/console/logs` (WebSocket)

Query: `level` (minimum `debug|info|warn|error`, default `info`), `since` (a `seq`). The server replays the last 500 records (those after `since` when given), sends one `{"type":"ready","seq":N}` and then every new record. `ready` carries `"reset":true` when `since` is newer than the newest `seq`: the daemon restarted, and the last 500 are sent instead.

Frame: `{"type":"log","seq":412,"time":"<RFC3339Nano>","level":"warn","component":"release","msg":"...","fields":{...}}`. `component` is the `component` attribute; groups are flattened to dotted keys.

Records live in a 5000-slot ring (`internal/core/logring`). A client that falls 256 records behind is disconnected; it reconnects with `since=<last seq>` and the replay closes the gap. Values of keys matching `token|secret|password|passwd|authorization|api key|private key|bearer|cookie|credential|pairing` are stored as `[redacted]`; string values are cut at 2 KiB and a record keeps 32 attributes.

## Commands: `GET /v0/console/commands`

`{"success":true,"data":{"commands":[{"path":["install"],"short":"...","usage":"install <ns> [flags]"}]}}`: only the commands the console may run.

## Exec: `POST /v0/console/exec`

Body `{"line":"install github.com/x/y"}`. Errors before execution are ordinary responses: 400 (empty, over 1024 bytes, control or shell characters), 403 (command not allowed; the message names it), 429 (4 commands already running). Otherwise a stream of `application/x-ndjson` frames: `{"type":"out","stream":"stdout|stderr","data":"..."}`, ending with exactly one `{"type":"exit","code":N,"error":""}`.

- **Allow rule.** A command runs only if it and every ancestor below the root carry `clierr.AllowInConsole`; the root and anything unmarked (`daemon`, `self-update`, `context`, `auth`, `completion`, `help`, hidden commands) are refused.
- **No shell, no quoting.** The line is rejected if it contains a control character or a quote (single or double), backtick, backslash or any of `$ ; & | < > ( ) { } * ? ~ #`; otherwise it is split on whitespace. Arguments containing spaces are not supported.
- **Session.** Each call builds a fresh command tree whose session dials the daemon's own address with the caller's bearer token and reads no CLI config, so `--server`, `--context` and `--config` have no effect.
- **Limits.** 10 minutes (the request context also cancels), 256 KiB of output then one truncation note, stdin is empty so confirmations answer no (destructive commands need `--yes`), a panic becomes `exit` code 1.
- **Audit.** Each call logs one `exec` line (component `console`, `device`, resolved command path, `code`, `took`; denials at warn). It is an intentional handler log, since a stream that has begun has nowhere to return an error.

Known limits: `status --watch` is not blocked and runs until the timeout or disconnect; the output cap counts bytes, not lines.
