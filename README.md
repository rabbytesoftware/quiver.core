<p align="center">
  <img src=".github/Quiver.png" alt="Quiver" width="300" />
  <br/>
  <em>Software distribution without a middleman.</em>
</p>

Software distribution remains a fragmented and technically demanding process for both developers and end users. Existing solutions are platform-specific, require technical knowledge, and maintain some degree of centralized control — leaving a gap for an open, accessible, and truly cross-platform alternative.

Quiver is a truly decentralized, cross-platform, open-source application store. Developers publish software in under five minutes by hosting a single file (a declarative manifest) on any Git-compatible repository, with no approval process, no fees. Because the file lives alongside the source code and CI pipeline, publishing a new release becomes a natural part of the existing development workflow, and for open-source projects, the distribution manifest is as transparent, versioned, and auditable as the code itself. End users install any published application in two clicks through an intuitive visual interface, without technical knowledge. The platform runs as a local service on each machine, enabling remote management of multiple hosts from a single desktop interface.

## Decentralized identity

There is no central registry. A package's identity is its source — a Git-hosted namespace that resolves directly to its manifest.

```
github.com/valve/steamcmd              # always latest
github.com/valve/steamcmd@v1.4.2      # pinned to a git tag
github.com/char2cs/gaming.quiver/cs2  # Arrow inside a collection repository
```

Anyone with a Git repository can publish. No account, no approval, no reserved names.

## The Arrow manifest

An Arrow is a single YAML file — `arrow.yaml` — hosted at the root of any Git repository alongside the source code. The same schema describes anything: a CLI tool, a game server, a web service, a system daemon.

What differentiates them is the lifecycle. A tool has only `install`:

```yaml
schema: "arrow@v0"

metadata:
  name: My Tool
  version: 1.0.0

targets:
  "*":
    lifecycle:
      install:
        - type: fetch
          url: https://example.com/tool
          to: ./tool
      uninstall: []
```

A service adds `execute` and `stop`:

```yaml
targets:
  "*":
    lifecycle:
      install:
        - type: fetch
          url: https://example.com/server.tar.gz
          to: ./server.tar.gz
      execute:
        - type: run
          command: ./server --port ${PORT}
      stop:
        - type: signal
          signal: graceful
      uninstall: []

    variables:
      - name: PORT
        type: number
        default: 8080
```

No extra configuration files, no separate service definitions — one schema, any kind of software.

## Cross-platform targeting

A single manifest targets all six supported platforms — `linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`, `darwin/amd64`, `darwin/arm64` — using glob patterns. Only the fields that actually differ need to be specified per platform.

```yaml
targets:
  _common:         # abstract base — never selected at runtime
    lifecycle:
      execute:
        - type: run
          command: ./server
      stop:
        - type: signal
          signal: graceful

  "linux/*":
    base: _common  # inherit execute and stop
    lifecycle:
      install:
        - type: fetch
          url:
            linux/amd64: https://example.com/server-linux-amd64.tar.gz
            linux/arm64: https://example.com/server-linux-arm64.tar.gz
          to: ./server
      uninstall: []

  "darwin/*":
    base: _common
    lifecycle:
      install:
        - type: fetch
          url:
            darwin/amd64: https://example.com/server-darwin-amd64.tar.gz
            darwin/arm64: https://example.com/server-darwin-arm64.tar.gz
          to: ./server
      uninstall: []
```

Targets are resolved once at add-time across all platforms. At runtime, selecting the right target is a single lookup.

## Versioned coexistence

Dependencies are identified by `namespace@ref`, not just namespace. Two packages that depend on different versions of the same Arrow install them independently — no conflict resolution, no forced upgrades, no compatibility matrix.

```
app-a → github.com/valve/steamcmd@v1.2.3
app-b → github.com/valve/steamcmd@v2.0.0
```

Both versions live side by side. Uninstalling `app-a` cleans up `v1.2.3` only if nothing else depends on it.

---

## Contributing

Quiver is open-source and we welcome contributions from the community — whether that's publishing packages, improving core, or anything in between.

### Common commands

| Command | Description |
|---|---|
| `make install-tools` | Install required dev tools (`golangci-lint`, `gofumpt`, `goimports`, `swag`) |
| `make build` | Build the binary |
| `make run` | Start the daemon locally |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests |
| `make test-coverage` | Run tests with HTML coverage report |
| `make bench` | Run benchmarks and check for regressions |
| `make bench-update` | Regenerate the benchmark baseline |
| `make fmt` | Format code (`gofumpt` + `goimports`) |
| `make lint` | Run `golangci-lint` |

Before opening a PR, always run:

```bash
make pr-checks
```

This runs the full validation suite — formatting, linting, security, build, docs, tests, and benchmarks. Run `make help` to see all available targets.

---

## License

Quiver is licensed under the [GPL-3.0](LICENSE).

---

## Stay Connected

- [Rabbyte GitHub](https://github.com/rabbytesoftware)
- [char2cs](https://char2cs.net)
