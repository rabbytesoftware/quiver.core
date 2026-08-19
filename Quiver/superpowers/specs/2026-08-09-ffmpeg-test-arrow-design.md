# FFmpeg Test Arrow — Design

**Date:** 2026-08-09
**Status:** Approved for planning
**Repo:** `github.com/valentin-villa/quiver.arrow-ffmpeg` (new, public)

---

## 1. Purpose

Verify end to end that Quiver can drive an arrow through its full obligatory lifecycle —
`install`, `execute`, `stop`, `uninstall` — and invoke custom `methods:` in both the `ready`
and `running` states, against real third-party software hosted in its own public repository.

The existing demo manifests under `docs/templates/demo/` do not serve this purpose. Several
were never actually run; §5 documents three latent defects they contain, found while
validating this design against the engine source.

---

## 2. Choice of software

FFmpeg, wrapped as a **test-pattern streaming service**.

FFmpeg is a one-shot tool and has no natural `execute`/`stop` pair. The spec permits `execute`
without `stop` (arrow.md §8.3, "tools that run once and exit"), but that leaves nothing to stop
and no window in which to invoke a `running` method — the two things this exercise exists to
verify. Running FFmpeg as a continuous `lavfi` test-pattern generator streaming to a UDP port
turns it into a genuine long-lived service without needing any input media.

The decisive property: **`ffprobe` ships in the same tarball**. A `available_in: [running]`
method can probe the live stream, so the running-state method genuinely interacts with the
supervised process rather than merely re-invoking the binary. Among the candidates considered
(Caddy, Syncthing, NATS), FFmpeg was chosen for familiarity; it retains the running-state
interaction that motivated the alternatives.

### 2.1 Source of binaries

`BtbN/FFmpeg-Builds`, release tag `latest`:

| Platform | Asset |
|---|---|
| `linux/amd64` | `ffmpeg-n7.1-latest-linux64-gpl-7.1.tar.xz` |
| `linux/arm64` | `ffmpeg-n7.1-latest-linuxarm64-gpl-7.1.tar.xz` |

Static builds, no root, no system dependencies. Both URLs verified to return `200`.

The `latest` tag is mutable — its assets are rebuilt in place — but the URL never 404s. This is
the durability property that matters, and the one the community AppImage URLs in the
discord/chrome demo manifests lacked.

BtbN publishes no darwin build. See §3 on scope.

---

## 3. Scope

**`linux/*` only.** One concrete target, no `base:` inheritance, with an Overrideable `url:`
covering the amd64/arm64 split.

Windows would roughly double the manifest — `.zip` instead of `.tar.xz`, `.exe` suffixes,
a different extract command — to exercise compiler paths that cannot be run on the development
machine. Darwin has no upstream static build from this source. The compiler skips an OS with no
matching target (`ErrNoTargetForOS`), and `no_supported_platform` does not fire while at least
one OS compiles, so a Linux-only manifest is valid. Additional platforms are a follow-up once
the Linux path is proven.

**Kind: service.** The single compiled target declares `execute`, so `ServicePackageRule`
infers a service. No mixing, no ambiguity.

**No `update:` hook.** Omitted deliberately; `quiver update` falls back to uninstall plus
reinstall. This is destructive and will be noted in the repository README.

---

## 4. Manifest design

Repository layout: a single `arrow.yaml` at the root, default branch `main`, matching the
existing `quiver.arrow-discord` test arrow.

```yaml
schema: "arrow@v0"

metadata:
  name: ffmpeg.teststream
  description: FFmpeg test-pattern stream server
  license: GPL-3.0
  url: https://ffmpeg.org
  maintainers:
    - name: Valentin Villa
  credits:
    - name: FFmpeg developers
      url: https://ffmpeg.org
    - name: BtbN
      url: https://github.com/BtbN/FFmpeg-Builds
  tags: [ffmpeg, video, streaming, test]

variables:
  - name: RESOLUTION
    type: select
    default: "640x480"
    values: ["320x240", "640x480", "1280x720"]
    description: Test pattern frame size

  - name: FRAMERATE
    type: number
    default: "25"
    min: 1
    max: 60
    description: Test pattern frame rate, also used as the keyframe interval

netbridge:
  - name: STREAM_PORT
    protocol: udp
    default: 23000
    required: true

targets:
  "linux/*":
    requirements:
      cpu_cores: 1
      ram_gb: 1
      disk_gb: 2

    lifecycle:
      install:
        - type: fetch
          url:
            linux/amd64: https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n7.1-latest-linux64-gpl-7.1.tar.xz
            linux/arm64: https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n7.1-latest-linuxarm64-gpl-7.1.tar.xz
          to: ./ffmpeg.tar.xz
          title: Downloading FFmpeg static build
          timeout: 10m

        - type: run
          command: tar -xJf ./ffmpeg.tar.xz --strip-components=2 --wildcards '*/bin/ffmpeg' '*/bin/ffprobe'
          title: Extracting ffmpeg and ffprobe
          timeout: 5m

        - type: run
          command: chmod +x ./ffmpeg ./ffprobe
          title: Setting executable bits
          timeout: 10s

      execute:
        - type: run
          command: ./ffmpeg -hide_banner -loglevel warning -re -f lavfi -i testsrc2=size=${RESOLUTION}:rate=${FRAMERATE} -c:v libx264 -preset ultrafast -tune zerolatency -g ${FRAMERATE} -f mpegts udp://127.0.0.1:${STREAM_PORT}
          title: Starting test-pattern stream
          exit_on_failure: false

      stop:
        - type: signal
          signal: graceful
          timeout: 15s
          exit_on_failure: false

      uninstall:
        - type: run
          command: rm -f ./ffmpeg ./ffprobe ./ffmpeg.tar.xz
          title: Removing FFmpeg
          timeout: 30s
          exit_on_failure: false

    methods:
      version:
        available_in: [ready, running]
        steps:
          - type: run
            command: ./ffmpeg -hide_banner -version
            title: Reporting FFmpeg version
            timeout: 30s

      list-encoders:
        available_in: [ready]
        steps:
          - type: run
            command: ./ffmpeg -hide_banner -encoders
            title: Listing available encoders
            timeout: 30s

      probe-stream:
        available_in: [running]
        steps:
          - type: run
            command: ./ffprobe -hide_banner -v fatal -select_streams v:0 -show_entries stream=codec_name,width,height,avg_frame_rate -of flat udp://127.0.0.1:${STREAM_PORT}
            title: Probing the live stream
            timeout: 30s
```

### 4.1 What each element exercises

| Element | Verifies |
|---|---|
| `fetch` with Overrideable `url:` | Per-arch resolution and coverage rules |
| `tar` + `chmod` run steps | Multi-step install, `${INSTALL_PATH}` as cwd |
| `execute` with no `timeout` | Unbounded supervised process — see §5.1 |
| `stop` as `type: signal` | `SignalPID` against the recorded PID |
| `version` in `[ready, running]` | Method dispatch in both states |
| `list-encoders` in `[ready]` | Correct rejection when invoked while running |
| `probe-stream` in `[running]` | Correct rejection when ready, and live process interaction |
| `RESOLUTION` / `FRAMERATE` | `select` and `number` variable validation |
| `STREAM_PORT` | Netbridge allocation and reference from a method — see §5.2 |

### 4.2 Note on quoting in the `tar` step

The single quotes around `'*/bin/ffmpeg'` are correct and deliberate: they stop the shell from
glob-expanding the pattern so that `tar --wildcards` receives it literally.

This is the opposite of the `${WORKDIR}` rule learned from the firefox arrow, where single
quotes wrongly deferred expansion and wrote a literal `${WORKDIR}` into a launcher script. The
distinction: suppress expansion when the *callee* must interpret the pattern; force expansion
when the value must be baked in at install time.

---

## 5. Engine findings

Three defects were found in the engine while validating this design. All three are pre-existing
and independent of this arrow; each is recorded here because it constrains the manifest above.

### 5.1 `timeout` on a `run` step caps process lifetime, not startup

`internal/engine/wizard/internal/step/run/handler.go:29-48` builds `stepCtx` with
`context.WithTimeout` and passes it to `proc.Wait(stepCtx)`. The timeout therefore bounds the
entire process lifetime. `handler_test.go::TestHandler_Execute_Timeout` confirms this: `sleep 10`
under a `50ms` timeout returns an error.

Consequently **every `execute` step in the existing demo templates would have its server killed
at the deadline** — `cs2-server.yaml` at `30s`, `firefox.yaml` at `15s`. Those execute paths
were evidently never run.

Mitigation here: omit `timeout` on the `execute` step. `timeout` is optional for `run`, and
when absent `stepCtx = ctx`, so the process runs until the parent context is cancelled.

### 5.2 Netbridge ports leak on every `Assemble`, and are pinned by `LastReturn`

`assembler/internal/variables.go:102-120` (Layer 4) calls `nb.Allocate` on *every* `Assemble`,
including method invocations. `netbridge.go:93-123` shows `Allocate` is **not** idempotent per
owner: it calls `findAvailablePort(preferred, ...)`, and since the previously allocated port is
already held, each call returns a *different* port and appends a new allocation.

The value the manifest actually sees is then overwritten by Layer 5
(`variables.go:122-126`), which copies `runtime.LastReturn.Variables` — and
`commands/end_execution.go:37` stores the full resolved variable map. So `${STREAM_PORT}` is in
practice pinned to the port allocated during **install**, and stays consistent across `execute`
and any `running` method.

Net effect for this arrow: `probe-stream` will observe the same port the stream is publishing
to, so the design works. But it works by accident of Layer 5 masking Layer 4, and every
`Assemble` leaks an allocation. Worth filing separately.

### 5.3 No way to express "termination by signal is success"

`wizard.go:177` fails an execution when a step errors and `ExitOnFailure()` is true, which is
the default. When `stop` signals the stream, FFmpeg exits non-zero, `run` returns
`ErrNonZeroExit`, and the execution outcome becomes `Failed` rather than a clean stop.

Setting `exit_on_failure: false` on the `execute` step avoids this. The cost is real and
accepted: a genuine *startup* failure — bad arguments, missing codec — will also be swallowed,
surfacing as an immediate transition back to `ready` rather than a reported failure.

The underlying gap is that the wizard cannot distinguish a supervised daemon, for which
signal-termination is the expected end, from a step that must exit zero.

---

## 6. Prototyping results

Every command was run against the real BtbN static build before being committed. Three
corrections came out of it, all now folded into §4.

**Keyframe interval.** The first `probe-stream` attempt failed: joining the MPEG-TS mid-flight,
ffprobe never saw the SPS/PPS headers and emitted an unbounded stream of `non-existing PPS 0
referenced`. x264's default keyframe interval leaves headers up to ten seconds apart. Pinning
`-g ${FRAMERATE}` forces one keyframe per second so headers repeat in-band. This is load-bearing
for the whole design — without it the running-state method cannot work at all.

**Log level.** Even at `-v error` the h264 decoder floods stderr while syncing. `-v fatal`
silences it without hiding a genuine failure, since the check is the exit code plus the
structured output.

**Confirmations.** FFmpeg survives streaming UDP to localhost with no listener attached (the
ICMP port-unreachable concern was unfounded). `tar --strip-components=2 --wildcards` extracts
both binaries correctly, already mode 755. SIGTERM produces **exit code 255**, directly
confirming §5.3. A probe returns in ~6s, well inside the `30s` method timeout, and reports
dimensions and frame rate matching whatever `RESOLUTION` and `FRAMERATE` were set to.

The download is ~115 MB and expands to two ~134 MB binaries, so `disk_gb` is 2, not 1.

The manifest validates against `translator/arrow/v0/schema.json`.

## 7. Verification plan

Against the daemon:

1. `arrow add` the namespace, confirm the manifest compiles and only `linux/*` targets appear.
2. `install` — confirm the fetch, extract, and chmod steps each report progress and the arrow
   reaches `ready`.
3. Invoke `version` and `list-encoders` in `ready`; confirm `probe-stream` is **rejected**.
4. `run` — confirm the arrow reaches `running` and stays there past 30 seconds, which is the
   direct test of §5.1.
5. Invoke `version` and `probe-stream` in `running`; confirm `probe-stream` reports the stream's
   resolution and framerate, and that `list-encoders` is **rejected**.
6. `stop` — confirm a clean return to `ready`.
7. `uninstall` — confirm return to `absent` and workdir cleanup.
8. Re-install with non-default `RESOLUTION` and `FRAMERATE` overrides; confirm `probe-stream`
   reflects them.

Step 5 is the payload: it is the first time a Quiver method will have interacted with a live
supervised process.

---

## 8. Out of scope

- Windows and darwin targets, and therefore `base:` inheritance.
- An `update:` hook.
- `tools:`, `services:`, and `exports:` — this is a single standalone arrow with no
  arrow-to-arrow relationships.
- Fixing any of the three engine defects in §5. They are recorded, not addressed.
