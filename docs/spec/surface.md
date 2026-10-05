# Quiver Surface Engine

## Overview

`internal/engine/surface` owns how an arrow's interface is reached. An arrow opens an interface by giving a `run` step a `ui` node ([manifests/v0/arrow.md §8.5](manifests/v0/arrow.md)); the wizard reports it as a `surface` event when the run starts and a `surface.closed` event when the run exits ([wizard.md](wizard.md)); the runtime repository records it on `Execution.Surface` and clears it again ([domain.md](domain.md)); and `GET/ANY /v0/ui/{ns}/*path` serves it ([http-api.md §6.3.1](http-api.md)). The engine does the provisioning, the readiness check and the serving. It opens no TCP listener: everything is reached through the daemon's own HTTP server and its bearer gate.

The engine is stateless apart from a per-socket proxy cache, and imports no other engine.

## Interface

| Method | Behavior |
|--------|----------|
| `SocketPath(ns)` | The deterministic socket address for `ns`. Touches nothing on disk |
| `Prepare(ns)` | Makes the run directory private, clears a stale socket and returns the address |
| `Cleanup(ns)` | Closes the cached proxy's idle connections and removes the socket file |
| `Handler(spec)` | The `http.Handler` serving the surface `spec` describes |
| `Ready(ctx, spec, path)` | Whether the surface answers a request for `path` |

`Spec` carries the `Mode` (`listen` or `static`), the `Namespace` and, for static mode, the absolute `Dir`.

## Socket address

v0 provisions a unix socket only. The `run_ui` rule therefore rejects a `listen` list without `unix` (`[pipe]` alone), so an arrow is never handed a filesystem socket path it asked to receive as a pipe name.

The address is `<run dir>/<12 hex>.sock`, where the 12 hex characters are the first six bytes of `sha256(namespace)`. It is deterministic, so the same namespace always maps to the same socket. A unix socket path is limited by `sockaddr_un` (about 104 bytes on darwin, 108 on linux), so `Prepare` refuses an address longer than 100 bytes with `ErrPathTooLong`. Hashing keeps the file name short whatever the namespace length; only a deeply nested run directory can trip the limit.

## Prepare and Cleanup

`Prepare` is called by the runtime assembler while it resolves variables, only for a method with a `run` step whose `ui` listens (the default when `ui` sets neither `listen` nor `static`). One address per namespace serves every such run of the method, since runs are sequential. It creates the run directory with mode `0700` (and re-applies the mode on an existing one), then decides about an existing socket file:

- If something still answers on it (a dial succeeds within 200 ms), the socket is left alone. A rejected duplicate `execute` therefore cannot break a live arrow.
- Otherwise the stale file is removed so the arrow can bind.

The returned address becomes `${ARROW_UI_LISTEN}`.

`Cleanup` runs when the run that opened the surface exits, right after `Execution.Surface` is cleared, and once more when the execution ends unless the run's own close already released the socket (so a lost `surface.closed` event, or a `stop` execution replacing the run, still leaves no socket behind). It drops the cached proxy for the socket, closes its idle connections and removes the socket file. Removing a file that is already gone is a no-op, so a second call is harmless.

## Readiness

The wizard reports a `listen` surface as not ready: the arrow still has to start and bind. The runtime repository starts a probe for it when the `surface` event arrives. The probe's lifetime is the run's: it stops when that surface is gone, whether the run exited or something replaced it.

- it asks the engine whether the surface answers a `GET` for the surface path (anything speaking HTTP counts, whatever the status; redirects are not followed; each attempt times out after 2 s), first after 250 ms and then backing off by doubling up to one attempt every 2 s;
- before each attempt it reads the runtime aggregate and stops, logging at debug level only, once the execution that opened the surface has ended or another execution (a stop, a new run) has replaced it;
- there is no other deadline: while that surface is open the probe keeps polling, so an arrow that starts slowly (a first run, a model load) still becomes ready;
- when one answers, `Ready` is recorded on the execution and broadcast like any other runtime change. If the execution is gone by then (the command is rejected as superseded or without an execution, or the store is shutting down), the result is dropped at debug level.

A `static` surface is reported ready by the wizard and `Ready` answers true without probing.

## Serving

**Listen mode** reverse-proxies to the arrow's unix socket. The outgoing request targets `http://localhost` with the incoming path and the query as net/http sanitised it (unparsable parameters, such as ones containing a semicolon, are dropped), the `Authorization` and `Cookie` headers are deleted, compression is not negotiated, and the response is flushed as it arrives, so streaming bodies work. WebSocket upgrades are handled by the same proxy. A dial or transport failure answers `502 arrow surface unavailable`. One proxy, with one bounded connection pool (4 idle connections, 90 s idle timeout), exists per socket path: the engine caches it and `Cleanup` evicts it.

**Static mode** serves a directory read-only (`GET` and `HEAD`, 405 otherwise). The directory is opened as an `os.Root` for each request, so a symlink pointing outside it cannot be followed. A request for `/` serves `index.html`, a directory serves its `index.html`, and a missing path without a file extension falls back to the root `index.html` so single-page apps can route on the client. A missing file with an extension is a 404.

## Lifetime

The surface exists exactly while the process of the `run` step that declared it runs, in any method that runs steps (`install`, `update`, `execute`, `stop`, `uninstall`, custom methods). It is opened before the process starts and closed when the process exits, whatever the cause: any exit code, a signal, a failure to start, a timeout, a cancel. The method then continues without it, and a later `run` of the same method may open its own.

- While open it is `active_run.surface` on the runtime and `GET/ANY /v0/ui/{ns}/*path` serves it. Once the run exits the field is gone and the route answers `503`.
- On a failed run the surface is closed before the step is reported failed, and a failed run still ends the method per its `exit_on_failure`.
- `stop` replaces the running execution: the surface leaves `active_run` the moment the stop begins, because the stop is a new execution with no surface of its own, and the socket is removed when the stop execution ends. The old run's drain is superseded and never touches the new execution or its surface.
- A daemon shutdown or crash leaves the socket file on disk. The next `Prepare` for the namespace clears a socket nothing answers on.

A `static` surface needs a run to live: the run is only the lifetime anchor and has to keep running (for example `sleep infinity`). A run that exits at once closes the surface at once.

## Errors

| Error | Meaning |
|-------|---------|
| `ErrPathTooLong` | The socket address exceeds 100 bytes |
| `ErrNoSurface` | The spec names no servable mode |
| `usecases.ErrNoSurface` | The arrow has no open surface (mapped to 503 by the route) |
